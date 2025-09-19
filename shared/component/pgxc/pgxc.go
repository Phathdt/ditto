package pgxc

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"github.com/jackc/pglogrepl"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	sctx "github.com/phathdt/service-context"
)

type PgxComp interface {
	GetConn() *pgconn.PgConn
	GetIdentity() pglogrepl.IdentifySystemResult
	GetLsn() pglogrepl.LSN
	GetDsn() string
}

type pgxc struct {
	id       string
	dbDsn    string
	slotName string
	logger   sctx.Logger
	conn     *pgconn.PgConn
	sysident pglogrepl.IdentifySystemResult
	lsn      pglogrepl.LSN
}

func New(id string) *pgxc {
	return &pgxc{id: id}
}

func (p *pgxc) ID() string {
	return p.id
}

func (p *pgxc) InitFlags() {
	flag.StringVar(&p.dbDsn, "db_dsn", "postgres://username:password@localhost:5432/database_name", "database dsn")
	flag.StringVar(&p.slotName, "slot_name", "ditto", "replication slot name")
}

func (p *pgxc) Activate(sc sctx.ServiceContext) error {
	p.logger = sc.Logger(p.id)

	queryDbDsn := strings.ReplaceAll(p.dbDsn, "&replication=database&", "&")
	queryDbDsn = strings.ReplaceAll(queryDbDsn, "?replication=database", "")
	queryDbDsn = strings.ReplaceAll(queryDbDsn, "?replication=database&", "&")
	queryDbDsn = strings.ReplaceAll(queryDbDsn, "&replication=database", "")

	queryConn, err := pgx.Connect(context.Background(), queryDbDsn)
	if err != nil {
		return err
	}

	var countPub int
	if err = queryConn.QueryRow(context.Background(), "SELECT COUNT(*) from pg_publication WHERE pubname = 'ditto'").
		Scan(&countPub); err != nil {
		return err
	}

	pubCon, err := pgconn.Connect(context.Background(), p.dbDsn)
	if err != nil {
		return err
	}

	// Use configurable slot name, default to "ditto" if not set
	slotName := p.slotName
	if slotName == "" {
		slotName = "ditto"
	}

	pluginArguments := []string{"proto_version '1'", fmt.Sprintf("publication_names '%s'", slotName)}

	var countSlot int
	if err = queryConn.QueryRow(context.Background(), "SELECT COUNT(*) FROM pg_replication_slots where slot_name = $1", slotName).Scan(&countSlot); err != nil {
		return err
	}

	if countSlot == 0 {
		_, err = pglogrepl.CreateReplicationSlot(context.Background(), pubCon, slotName, "pgoutput", pglogrepl.CreateReplicationSlotOptions{Temporary: false})
		if err != nil {
			return fmt.Errorf("CreateReplicationSlot failed: %w", err)
		}
	}

	var restartLSNStr string

	if err = queryConn.QueryRow(context.Background(), "SELECT restart_lsn FROM pg_replication_slots WHERE slot_name = $1", slotName).
		Scan(&restartLSNStr); err != nil {
		return err
	}

	lsn, err := pglogrepl.ParseLSN(restartLSNStr)
	if err != nil {
		return err
	}
	p.lsn = lsn

	p.conn = pubCon

	err = pglogrepl.StartReplication(context.Background(), pubCon, slotName, p.lsn, pglogrepl.StartReplicationOptions{PluginArgs: pluginArguments})
	if err != nil {
		return err
	}
	p.logger.Infoln("Logical replication started on slot", slotName)

	return nil
}

func (p *pgxc) Stop() error {
	return p.conn.Close(context.Background())
}

func (p *pgxc) GetConn() *pgconn.PgConn {
	return p.conn
}

func (p *pgxc) GetIdentity() pglogrepl.IdentifySystemResult {
	return p.sysident
}

func (p *pgxc) GetLsn() pglogrepl.LSN {
	return p.lsn
}

func (p *pgxc) GetDsn() string {
	return p.dbDsn
}
