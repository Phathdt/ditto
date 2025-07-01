package listener

import (
	"context"
	"ditto/listener/parsers"
	"ditto/models"
	"ditto/shared/common"
	"ditto/shared/component/pgxc"
	"ditto/shared/component/redisc"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/jackc/pglogrepl"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgproto3"
	sctx "github.com/phathdt/service-context"
)

type Parser interface {
	ParseWalMessage([]byte, *models.WalTransaction) error
}

type Config struct {
	WatchList           map[string]models.WatchConfig `yaml:"watch_list"`
	PrefixWatchList     string                        `yaml:"prefix_watch_list"`
	PublicationStrategy string                        `yaml:"publication_strategy"` // "single" or "multiple"
	PublicationPrefix   string                        `yaml:"publication_prefix"`   // prefix for multiple publications
}

type listener struct {
	conn      *pgconn.PgConn
	sysident  pglogrepl.IdentifySystemResult
	logger    sctx.Logger
	parser    Parser
	mu        sync.RWMutex
	lsn       pglogrepl.LSN
	publisher redisc.RedisComp
	dbDsn     string
}

func New(sc sctx.ServiceContext) *listener {
	conn := sc.MustGet(common.KeyCompPgx).(pgxc.PgxComp).GetConn()
	sysident := sc.MustGet(common.KeyCompPgx).(pgxc.PgxComp).GetIdentity()
	lsn := sc.MustGet(common.KeyCompPgx).(pgxc.PgxComp).GetLsn()
	publisher := sc.MustGet(common.KeyCompRedis).(redisc.RedisComp)
	logger := sc.Logger("global")
	dbDsn := sc.MustGet(common.KeyCompPgx).(pgxc.PgxComp).GetDsn()

	parser := parsers.NewBinaryParser(binary.BigEndian)

	return &listener{conn: conn, sysident: sysident, lsn: lsn, logger: logger, parser: parser, publisher: publisher, dbDsn: dbDsn}
}

func (l *listener) Process() error {
	clientXLogPos := l.sysident.XLogPos
	standbyMessageTimeout := time.Second * 10
	nextStandbyMessageDeadline := time.Now().Add(standbyMessageTimeout)

	f, err := os.Open("config/config.yml")
	if err != nil {
		return err
	}
	defer f.Close()
	var cfg Config
	if err := yaml.NewDecoder(f).Decode(&cfg); err != nil {
		return err
	}

	if err := l.createPublicationFromConfig(cfg); err != nil {
		return err
	}

	tx := models.NewWalTransaction()

	for {
		if time.Now().After(nextStandbyMessageDeadline) {
			err := pglogrepl.SendStandbyStatusUpdate(context.Background(), l.conn, pglogrepl.StandbyStatusUpdate{WALWritePosition: clientXLogPos})
			if err != nil {
				l.logger.Fatalln("SendStandbyStatusUpdate failed:", err)
			}
			l.logger.Infoln("Sent Standby status message")
			nextStandbyMessageDeadline = time.Now().Add(standbyMessageTimeout)
		}

		ctx, cancel := context.WithDeadline(context.Background(), nextStandbyMessageDeadline)
		rawMsg, err := l.conn.ReceiveMessage(ctx)
		cancel()
		if err != nil {
			if pgconn.Timeout(err) {
				continue
			}
			l.logger.Fatalln("ReceiveMessage failed:", err)
		}

		if errMsg, ok := rawMsg.(*pgproto3.ErrorResponse); ok {
			l.logger.Fatalf("received Postgres WAL error: %+v", errMsg)
		}

		msg, ok := rawMsg.(*pgproto3.CopyData)
		if !ok {
			l.logger.Infof("Received unexpected message: %T\n", rawMsg)
			continue
		}

		switch msg.Data[0] {
		case pglogrepl.PrimaryKeepaliveMessageByteID:
			pkm, err := pglogrepl.ParsePrimaryKeepaliveMessage(msg.Data[1:])
			if err != nil {
				l.logger.Fatalln("ParsePrimaryKeepaliveMessage failed:", err)
			}
			l.logger.Infoln("Primary Keepalive Message =>", "ServerWALEnd:", pkm.ServerWALEnd, "ServerTime:", pkm.ServerTime, "ReplyRequested:", pkm.ReplyRequested)

			if pkm.ReplyRequested {
				nextStandbyMessageDeadline = time.Time{}
			}

		case pglogrepl.XLogDataByteID:
			xld, err := pglogrepl.ParseXLogData(msg.Data[1:])
			if err != nil {
				l.logger.Fatalln("ParseXLogData failed:", err)
				continue
			}

			if err = l.parser.ParseWalMessage(xld.WALData, tx); err != nil {
				l.logger.Fatalln("ParseWalMessage failed:", err)
				continue
			}

			if tx.CommitTime != nil {
				events := tx.CreateEventsWithWatchList(cfg.WatchList)
				for _, event := range events {
					topic := buildTopic(cfg.PrefixWatchList, event.Table, cfg.WatchList)
					if err := l.publisher.Publish(topic, event); err != nil {
						l.logger.Errorln("Failed to publish event:", err)
					}
				}
				tx.Clear()
			}

			if xld.WALStart > l.readLSN() {
				if err = l.AckWalMessage(xld.WALStart); err != nil {
					l.logger.Errorf("acknowledge wal message: %w", err)
					continue
				}
				l.logger.Infof("lsn = %d ack wal msg", l.readLSN())
			}
			clientXLogPos = xld.WALStart + pglogrepl.LSN(len(xld.WALData))
		}
	}
}

func buildTopic(prefix, table string, watchList map[string]models.WatchConfig) string {
	mapping := table
	if w, ok := watchList[table]; ok && w.Mapping != "" {
		mapping = w.Mapping
	}
	if prefix != "" {
		return prefix + "." + mapping
	}
	return mapping
}

// SendStandbyStatus sends a `StandbyStatus` object with the current RestartLSN value to the server.
func (l *listener) SendStandbyStatus() error {
	standbyStatus := pglogrepl.StandbyStatusUpdate{
		WALWritePosition: l.readLSN(),
		ReplyRequested:   false,
	}
	if err := pglogrepl.SendStandbyStatusUpdate(context.Background(), l.conn, standbyStatus); err != nil {
		return fmt.Errorf("unable to create StandbyStatus object: %w", err)
	}

	return nil
}

// AckWalMessage acknowledge received wal message.
func (l *listener) AckWalMessage(lsn pglogrepl.LSN) error {
	l.setLSN(lsn)

	if err := l.SendStandbyStatus(); err != nil {
		return fmt.Errorf("send status: %w", err)
	}

	return nil
}

func (l *listener) readLSN() pglogrepl.LSN {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.lsn
}

func (l *listener) setLSN(lsn pglogrepl.LSN) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.lsn = lsn
}

func (l *listener) createPublicationFromConfig(cfg Config) error {
	sqlDsn := strings.ReplaceAll(l.dbDsn, "replication=database", "")
	sqlConn, err := pgx.Connect(context.Background(), sqlDsn)
	if err != nil {
		return err
	}
	defer sqlConn.Close(context.Background())

	tableNames := make([]string, 0, len(cfg.WatchList))
	for table := range cfg.WatchList {
		tableNames = append(tableNames, table)
	}

	fmt.Println("tableNames", tableNames)

	// Default to single publication strategy
	strategy := cfg.PublicationStrategy
	if strategy == "" {
		strategy = "single"
	}

	switch strategy {
	case "single":
		if err := l.ensurePublicationMatches(sqlConn, tableNames); err != nil {
			return err
		}
		l.logger.Infoln("single publication ditto is ready")

	case "multiple":
		prefix := cfg.PublicationPrefix
		if prefix == "" {
			prefix = "ditto"
		}
		if err := l.ensureMultiplePublications(sqlConn, tableNames, prefix); err != nil {
			return err
		}
		l.logger.Infoln("multiple publications are ready")

	default:
		return fmt.Errorf("unsupported publication strategy: %s", strategy)
	}

	return nil
}

func (l *listener) ensurePublicationMatches(conn *pgx.Conn, expectedTables []string) error {
	publicationName := "ditto"

	// Check if publication exists and get current tables
	currentTables, err := l.getCurrentPublicationTables(conn, publicationName)
	if err != nil {
		return fmt.Errorf("failed to get current publication tables: %w", err)
	}

	// Compare expected vs current tables
	if l.tablesMatch(expectedTables, currentTables) {
		l.logger.Infof("publication %s already matches expected tables", publicationName)
		return nil
	}

	l.logger.Infof("publication %s doesn't match expected tables, recreating...", publicationName)

	// Drop existing publication
	dropSQL := fmt.Sprintf("DROP PUBLICATION IF EXISTS %s;", publicationName)
	if _, err := conn.Exec(context.Background(), dropSQL); err != nil {
		return fmt.Errorf("failed to drop publication: %w", err)
	}

	// Create new publication
	var sqlTable string
	if len(expectedTables) == 0 {
		sqlTable = "FOR ALL TABLES"
	} else {
		sqlTable = "FOR TABLE " + strings.Join(expectedTables, ", ")
	}

	createSQL := fmt.Sprintf("CREATE PUBLICATION %s %s;", publicationName, sqlTable)
	l.logger.Infof("creating publication with SQL: %s", createSQL)

	if _, err := conn.Exec(context.Background(), createSQL); err != nil {
		return fmt.Errorf("failed to create publication: %w", err)
	}

	l.logger.Infof("successfully recreated publication %s", publicationName)
	return nil
}

func (l *listener) getCurrentPublicationTables(conn *pgx.Conn, publicationName string) ([]string, error) {
	query := `
		SELECT COALESCE(array_agg(t.tablename ORDER BY t.tablename), '{}') as tables
		FROM pg_publication p
		LEFT JOIN pg_publication_tables t ON p.pubname = t.pubname
		WHERE p.pubname = $1
		GROUP BY p.pubname;
	`

	var tables []string
	err := conn.QueryRow(context.Background(), query, publicationName).Scan(&tables)
	if err != nil {
		if err == pgx.ErrNoRows {
			return []string{}, nil
		}
		return nil, err
	}

	// Filter out empty strings that might come from LEFT JOIN
	var result []string
	for _, table := range tables {
		if table != "" {
			result = append(result, table)
		}
	}

	return result, nil
}

func (l *listener) tablesMatch(expected, current []string) bool {
	if len(expected) != len(current) {
		return false
	}

	// Create maps for comparison
	expectedMap := make(map[string]bool)
	for _, table := range expected {
		expectedMap[table] = true
	}

	currentMap := make(map[string]bool)
	for _, table := range current {
		currentMap[table] = true
	}

	// Check if all expected tables are in current
	for table := range expectedMap {
		if !currentMap[table] {
			return false
		}
	}

	// Check if all current tables are in expected
	for table := range currentMap {
		if !expectedMap[table] {
			return false
		}
	}

	return true
}

func (l *listener) ensureMultiplePublications(conn *pgx.Conn, tables []string, prefix string) error {
	for _, table := range tables {
		publicationName := fmt.Sprintf("%s_%s", prefix, table)

		// Check if publication exists for this table
		currentTables, err := l.getCurrentPublicationTables(conn, publicationName)
		if err != nil {
			return fmt.Errorf("failed to get current publication tables for %s: %w", publicationName, err)
		}

		expectedTables := []string{table}

		// Compare expected vs current tables
		if l.tablesMatch(expectedTables, currentTables) {
			l.logger.Infof("publication %s already matches expected table", publicationName)
			continue
		}

		l.logger.Infof("publication %s doesn't match, recreating for table %s", publicationName, table)

		// Drop existing publication
		dropSQL := fmt.Sprintf("DROP PUBLICATION IF EXISTS %s;", publicationName)
		if _, err := conn.Exec(context.Background(), dropSQL); err != nil {
			return fmt.Errorf("failed to drop publication %s: %w", publicationName, err)
		}

		// Create new publication for single table
		createSQL := fmt.Sprintf("CREATE PUBLICATION %s FOR TABLE %s;", publicationName, table)
		l.logger.Infof("creating publication with SQL: %s", createSQL)

		if _, err := conn.Exec(context.Background(), createSQL); err != nil {
			return fmt.Errorf("failed to create publication %s: %w", publicationName, err)
		}

		l.logger.Infof("successfully created publication %s for table %s", publicationName, table)
	}

	return nil
}
