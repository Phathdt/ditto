package listener

import (
	"context"
	"ditto/listener/parsers"
	"ditto/models"
	"ditto/shared/common"
	"ditto/shared/component/pgxc"
	"ditto/shared/component/watermillapp"
	"encoding/binary"
	"fmt"
	"github.com/goccy/go-json"
	"github.com/jackc/pglogrepl"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/namsral/flag"
	sctx "github.com/viettranx/service-context"
	"sync"
	"time"
)

type Parser interface {
	ParseWalMessage([]byte, *models.WalTransaction) error
}

type listener struct {
	conn      *pgconn.PgConn
	sysident  pglogrepl.IdentifySystemResult
	logger    sctx.Logger
	parser    Parser
	mu        sync.RWMutex
	lsn       pglogrepl.LSN
	publisher watermillapp.Publisher
}

func New(sc sctx.ServiceContext) *listener {
	conn := sc.MustGet(common.KeyCompPgx).(pgxc.PgxComp).GetConn()
	sysident := sc.MustGet(common.KeyCompPgx).(pgxc.PgxComp).GetIdentity()
	lsn := sc.MustGet(common.KeyCompPgx).(pgxc.PgxComp).GetLsn()
	publisher := sc.MustGet(common.KeyCompNatsPub).(watermillapp.Publisher)
	logger := sc.Logger("global")

	parser := parsers.NewBinaryParser(binary.BigEndian)

	return &listener{conn: conn, sysident: sysident, lsn: lsn, logger: logger, parser: parser, publisher: publisher}
}

func (l *listener) Process() error {
	clientXLogPos := l.sysident.XLogPos
	standbyMessageTimeout := time.Second * 10
	nextStandbyMessageDeadline := time.Now().Add(standbyMessageTimeout)

	var tableFilterRaw string
	var topicMappingRaw string
	flag.StringVar(&tableFilterRaw, "table_filter", "", "filter messages by table")
	flag.StringVar(&topicMappingRaw, "topic_mapping", "", "mapping topic")

	flag.Parse()
	tableFilter := make(map[string][]string)
	if err := json.Unmarshal([]byte(tableFilterRaw), &tableFilter); err != nil {
		return err
	}

	topicMapping := make(map[string]string)
	if err := json.Unmarshal([]byte(topicMappingRaw), &topicMapping); err != nil {
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
				events := tx.CreateEventsWithFilter(tableFilter)
				for _, event := range events {
					fmt.Println(">>>>>>>>>>>>>>>>>>>")
					if err = l.publisher.Publish(event.SubjectName(topicMapping), event); err != nil {
						l.logger.Errorln(err)
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
