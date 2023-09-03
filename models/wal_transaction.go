package models

import (
	"ditto/common"
	"ditto/errorx"
	"strconv"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// ActionKind kind of action on WAL message.
type ActionKind string

// kind of WAL message.
const (
	ActionKindInsert ActionKind = "INSERT"
	ActionKindUpdate ActionKind = "UPDATE"
	ActionKindDelete ActionKind = "DELETE"
)

// WalTransaction transaction specified WAL message.
type WalTransaction struct {
	LSN           int64
	BeginTime     *time.Time
	CommitTime    *time.Time
	RelationStore map[int32]RelationData
	Actions       []ActionData
}

// NewWalTransaction create and initialize new WAL transaction.
func NewWalTransaction() *WalTransaction {
	return &WalTransaction{
		RelationStore: make(map[int32]RelationData),
	}
}

func (k ActionKind) string() string {
	return string(k)
}

// RelationData kind of WAL message data.
type RelationData struct {
	Schema  string
	Table   string
	Columns []Column
}

// ActionData kind of WAL message data.
type ActionData struct {
	Schema     string
	Table      string
	Kind       ActionKind
	OldColumns []Column
	NewColumns []Column
}

// Column of the table with which changes occur.
type Column struct {
	Name      string
	value     any
	ValueType int
	IsKey     bool
}

// AssertValue converts bytes to a specific type depending
// on the type of this data in the database table.
func (c *Column) AssertValue(src []byte) {
	var (
		val any
		err error
	)

	if src == nil {
		c.value = nil
		return
	}

	strSrc := string(src)

	const (
		timestampLayout       = "2006-01-02 15:04:05"
		timestampWithTZLayout = "2006-01-02 15:04:05.999999999-07"
	)

	switch c.ValueType {
	case common.BoolOID:
		val, err = strconv.ParseBool(strSrc)
	case common.Int2OID, common.Int4OID:
		val, err = strconv.Atoi(strSrc)
	case common.Int8OID, common.Numeric:
		val, err = strconv.ParseInt(strSrc, 10, 64)
	case common.TextOID, common.VarcharOID:
		val = strSrc
	case common.TimestampOID:
		val, err = time.Parse(timestampLayout, strSrc)
	case common.TimestamptzOID:
		val, err = time.ParseInLocation(timestampWithTZLayout, strSrc, time.UTC)
	case common.DateOID, common.TimeOID:
		val = strSrc
	case common.UUIDOID:
		val, err = uuid.Parse(strSrc)
	case common.JSONBOID:
		var m any
		if src[0] == '[' {
			m = make([]any, 0)
		} else {
			m = make(map[string]any)
		}
		err = json.Unmarshal(src, &m)
		val = m
	default:
		logrus.WithFields(logrus.Fields{"pgtype": c.ValueType, "column_name": c.Name}).Warnln("unknown oid type")
		val = strSrc
	}

	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{"pgtype": c.ValueType, "column_name": c.Name}).
			Errorln("column data parse error")
	}

	c.value = val
}

// Clear transaction data.
func (w *WalTransaction) Clear() {
	w.CommitTime = nil
	w.BeginTime = nil
	w.Actions = nil
}

// CreateActionData create action  from WAL message data.
func (w *WalTransaction) CreateActionData(relationID int32, oldRows []common.TupleData, newRows []common.TupleData, kind ActionKind) (a ActionData, err error) {
	rel, ok := w.RelationStore[relationID]
	if !ok {
		return a, errorx.ErrRelationNotFound
	}

	a = ActionData{
		Schema: rel.Schema,
		Table:  rel.Table,
		Kind:   kind,
	}

	var oldColumns []Column

	for num, row := range oldRows {
		column := Column{
			Name:      rel.Columns[num].Name,
			ValueType: rel.Columns[num].ValueType,
			IsKey:     rel.Columns[num].IsKey,
		}
		column.AssertValue(row.Value)
		oldColumns = append(oldColumns, column)
	}

	a.OldColumns = oldColumns

	var newColumns []Column
	for num, row := range newRows {
		column := Column{
			Name:      rel.Columns[num].Name,
			ValueType: rel.Columns[num].ValueType,
			IsKey:     rel.Columns[num].IsKey,
		}
		column.AssertValue(row.Value)
		newColumns = append(newColumns, column)
	}
	a.NewColumns = newColumns

	return a, nil
}

// CreateEventsWithFilter filter WAL message by table,
// action and create events for each value.
func (w *WalTransaction) CreateEventsWithFilter(tableMap map[string][]string) []Event {
	var events []Event

	for _, item := range w.Actions {
		dataOld := make(map[string]any)
		for _, val := range item.OldColumns {
			dataOld[val.Name] = val.value
		}

		data := make(map[string]any)
		for _, val := range item.NewColumns {
			data[val.Name] = val.value
		}

		event := Event{
			ID:        uuid.New(),
			Schema:    item.Schema,
			Table:     item.Table,
			Action:    item.Kind.string(),
			DataOld:   dataOld,
			Data:      data,
			EventTime: *w.CommitTime,
		}

		actions, validTable := tableMap[item.Table]

		validAction := inArray(actions, item.Kind.string())
		if validTable && validAction {
			events = append(events, event)
			continue
		}

		logrus.WithFields(
			logrus.Fields{
				"schema": item.Schema,
				"table":  item.Table,
				"action": item.Kind,
			}).
			Infoln("wal-message was skipped by filter")
	}

	return events
}

func (w *WalTransaction) CreateEvents() []Event {
	var events []Event

	for _, item := range w.Actions {
		dataOld := make(map[string]any)
		for _, val := range item.OldColumns {
			dataOld[val.Name] = val.value
		}

		data := make(map[string]any)
		for _, val := range item.NewColumns {
			data[val.Name] = val.value
		}

		event := Event{
			ID:        uuid.New(),
			Schema:    item.Schema,
			Table:     item.Table,
			Action:    item.Kind.string(),
			DataOld:   dataOld,
			Data:      data,
			EventTime: *w.CommitTime,
		}

		events = append(events, event)
	}

	return events
}

// inArray checks whether the value is in an array.
func inArray(arr []string, value string) bool {
	for _, v := range arr {
		if strings.EqualFold(v, value) {
			return true
		}
	}

	return false
}
