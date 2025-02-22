// Code generated by pggen. DO NOT EDIT.

package models

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgtype"
	"github.com/jackc/pgx/v4"
)

// Querier is a typesafe Go interface backed by SQL queries.
//
// Methods ending with Batch enqueue a query to run later in a pgx.Batch. After
// calling SendBatch on pgx.Conn, pgxpool.Pool, or pgx.Tx, use the Scan methods
// to parse the results.
type Querier interface {
	GetCurrentScheduleForDevice(ctx context.Context, deviceID uuid.UUID) (GetCurrentScheduleForDeviceRow, error)
	// GetCurrentScheduleForDeviceBatch enqueues a GetCurrentScheduleForDevice query into batch to be executed
	// later by the batch.
	GetCurrentScheduleForDeviceBatch(batch genericBatch, deviceID uuid.UUID)
	// GetCurrentScheduleForDeviceScan scans the result of an executed GetCurrentScheduleForDeviceBatch query.
	GetCurrentScheduleForDeviceScan(results pgx.BatchResults) (GetCurrentScheduleForDeviceRow, error)

	GetContainersForSchedule(ctx context.Context, scheduleID uuid.UUID) ([]GetContainersForScheduleRow, error)
	// GetContainersForScheduleBatch enqueues a GetContainersForSchedule query into batch to be executed
	// later by the batch.
	GetContainersForScheduleBatch(batch genericBatch, scheduleID uuid.UUID)
	// GetContainersForScheduleScan scans the result of an executed GetContainersForScheduleBatch query.
	GetContainersForScheduleScan(results pgx.BatchResults) ([]GetContainersForScheduleRow, error)

	GetDeviceByName(ctx context.Context, name *string) (GetDeviceByNameRow, error)
	// GetDeviceByNameBatch enqueues a GetDeviceByName query into batch to be executed
	// later by the batch.
	GetDeviceByNameBatch(batch genericBatch, name *string)
	// GetDeviceByNameScan scans the result of an executed GetDeviceByNameBatch query.
	GetDeviceByNameScan(results pgx.BatchResults) (GetDeviceByNameRow, error)
}

type DBQuerier struct {
	conn  genericConn   // underlying Postgres transport to use
	types *typeResolver // resolve types by name
}

var _ Querier = &DBQuerier{}

// genericConn is a connection to a Postgres database. This is usually backed by
// *pgx.Conn, pgx.Tx, or *pgxpool.Pool.
type genericConn interface {
	// Query executes sql with args. If there is an error the returned Rows will
	// be returned in an error state. So it is allowed to ignore the error
	// returned from Query and handle it in Rows.
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)

	// QueryRow is a convenience wrapper over Query. Any error that occurs while
	// querying is deferred until calling Scan on the returned Row. That Row will
	// error with pgx.ErrNoRows if no rows are returned.
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row

	// Exec executes sql. sql can be either a prepared statement name or an SQL
	// string. arguments should be referenced positionally from the sql string
	// as $1, $2, etc.
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
}

// genericBatch batches queries to send in a single network request to a
// Postgres server. This is usually backed by *pgx.Batch.
type genericBatch interface {
	// Queue queues a query to batch b. query can be an SQL query or the name of a
	// prepared statement. See Queue on *pgx.Batch.
	Queue(query string, arguments ...interface{})
}

// NewQuerier creates a DBQuerier that implements Querier. conn is typically
// *pgx.Conn, pgx.Tx, or *pgxpool.Pool.
func NewQuerier(conn genericConn) *DBQuerier {
	return NewQuerierConfig(conn, QuerierConfig{})
}

type QuerierConfig struct {
	// DataTypes contains pgtype.Value to use for encoding and decoding instead
	// of pggen-generated pgtype.ValueTranscoder.
	//
	// If OIDs are available for an input parameter type and all of its
	// transitive dependencies, pggen will use the binary encoding format for
	// the input parameter.
	DataTypes []pgtype.DataType
}

// NewQuerierConfig creates a DBQuerier that implements Querier with the given
// config. conn is typically *pgx.Conn, pgx.Tx, or *pgxpool.Pool.
func NewQuerierConfig(conn genericConn, cfg QuerierConfig) *DBQuerier {
	return &DBQuerier{conn: conn, types: newTypeResolver(cfg.DataTypes)}
}

// WithTx creates a new DBQuerier that uses the transaction to run all queries.
func (q *DBQuerier) WithTx(tx pgx.Tx) (*DBQuerier, error) {
	return &DBQuerier{conn: tx}, nil
}

// preparer is any Postgres connection transport that provides a way to prepare
// a statement, most commonly *pgx.Conn.
type preparer interface {
	Prepare(ctx context.Context, name, sql string) (sd *pgconn.StatementDescription, err error)
}

// PrepareAllQueries executes a PREPARE statement for all pggen generated SQL
// queries in querier files. Typical usage is as the AfterConnect callback
// for pgxpool.Config
//
// pgx will use the prepared statement if available. Calling PrepareAllQueries
// is an optional optimization to avoid a network round-trip the first time pgx
// runs a query if pgx statement caching is enabled.
func PrepareAllQueries(ctx context.Context, p preparer) error {
	if _, err := p.Prepare(ctx, getCurrentScheduleForDeviceSQL, getCurrentScheduleForDeviceSQL); err != nil {
		return fmt.Errorf("prepare query 'GetCurrentScheduleForDevice': %w", err)
	}
	if _, err := p.Prepare(ctx, getContainersForScheduleSQL, getContainersForScheduleSQL); err != nil {
		return fmt.Errorf("prepare query 'GetContainersForSchedule': %w", err)
	}
	if _, err := p.Prepare(ctx, getDeviceByNameSQL, getDeviceByNameSQL); err != nil {
		return fmt.Errorf("prepare query 'GetDeviceByName': %w", err)
	}
	return nil
}

// typeResolver looks up the pgtype.ValueTranscoder by Postgres type name.
type typeResolver struct {
	connInfo *pgtype.ConnInfo // types by Postgres type name
}

func newTypeResolver(types []pgtype.DataType) *typeResolver {
	ci := pgtype.NewConnInfo()
	for _, typ := range types {
		if txt, ok := typ.Value.(textPreferrer); ok && typ.OID != unknownOID {
			typ.Value = txt.ValueTranscoder
		}
		ci.RegisterDataType(typ)
	}
	return &typeResolver{connInfo: ci}
}

// findValue find the OID, and pgtype.ValueTranscoder for a Postgres type name.
func (tr *typeResolver) findValue(name string) (uint32, pgtype.ValueTranscoder, bool) {
	typ, ok := tr.connInfo.DataTypeForName(name)
	if !ok {
		return 0, nil, false
	}
	v := pgtype.NewValue(typ.Value)
	return typ.OID, v.(pgtype.ValueTranscoder), true
}

// setValue sets the value of a ValueTranscoder to a value that should always
// work and panics if it fails.
func (tr *typeResolver) setValue(vt pgtype.ValueTranscoder, val interface{}) pgtype.ValueTranscoder {
	if err := vt.Set(val); err != nil {
		panic(fmt.Sprintf("set ValueTranscoder %T to %+v: %s", vt, val, err))
	}
	return vt
}

const getCurrentScheduleForDeviceSQL = `SELECT s.*
FROM device AS d
LEFT JOIN fleet_schedule AS fs ON fs.fleet_id = d.fleet_id
LEFT JOIN fleet AS f ON f.id = fs.fleet_id
LEFT JOIN schedule AS s ON s.id = f.default_schedule_id
WHERE d.id = $1;`

type GetCurrentScheduleForDeviceRow struct {
	ID        uuid.UUID  `json:"id"`
	Name      *string    `json:"name"`
	State     *string    `json:"state"`
	CreatedAt *time.Time `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`
}

// GetCurrentScheduleForDevice implements Querier.GetCurrentScheduleForDevice.
func (q *DBQuerier) GetCurrentScheduleForDevice(ctx context.Context, deviceID uuid.UUID) (GetCurrentScheduleForDeviceRow, error) {
	ctx = context.WithValue(ctx, "pggen_query_name", "GetCurrentScheduleForDevice")
	row := q.conn.QueryRow(ctx, getCurrentScheduleForDeviceSQL, deviceID)
	var item GetCurrentScheduleForDeviceRow
	if err := row.Scan(&item.ID, &item.Name, &item.State, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return item, fmt.Errorf("query GetCurrentScheduleForDevice: %w", err)
	}
	return item, nil
}

// GetCurrentScheduleForDeviceBatch implements Querier.GetCurrentScheduleForDeviceBatch.
func (q *DBQuerier) GetCurrentScheduleForDeviceBatch(batch genericBatch, deviceID uuid.UUID) {
	batch.Queue(getCurrentScheduleForDeviceSQL, deviceID)
}

// GetCurrentScheduleForDeviceScan implements Querier.GetCurrentScheduleForDeviceScan.
func (q *DBQuerier) GetCurrentScheduleForDeviceScan(results pgx.BatchResults) (GetCurrentScheduleForDeviceRow, error) {
	row := results.QueryRow()
	var item GetCurrentScheduleForDeviceRow
	if err := row.Scan(&item.ID, &item.Name, &item.State, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return item, fmt.Errorf("scan GetCurrentScheduleForDeviceBatch row: %w", err)
	}
	return item, nil
}

const getContainersForScheduleSQL = `SELECT c.*
FROM container AS c
WHERE c.schedule_id = $1;`

type GetContainersForScheduleRow struct {
	ID               uuid.UUID  `json:"id"`
	CreatedAt        *time.Time `json:"created_at"`
	UpdatedAt        *time.Time `json:"updated_at"`
	Name             *string    `json:"name"`
	ContainerImage   *string    `json:"container_image"`
	Env              []byte     `json:"env"`
	Privileged       bool       `json:"privileged"`
	NetworkMode      *string    `json:"network_mode"`
	Ports            []byte     `json:"ports"`
	BindDev          bool       `json:"bind_dev"`
	BindProc         bool       `json:"bind_proc"`
	BindSys          bool       `json:"bind_sys"`
	BindShm          bool       `json:"bind_shm"`
	BindCgroup       bool       `json:"bind_cgroup"`
	BindDockerSocket bool       `json:"bind_docker_socket"`
	BindBoot         bool       `json:"bind_boot"`
	Command          *string    `json:"command"`
	Entrypoint       *string    `json:"entrypoint"`
	ScheduleID       uuid.UUID  `json:"schedule_id"`
}

// GetContainersForSchedule implements Querier.GetContainersForSchedule.
func (q *DBQuerier) GetContainersForSchedule(ctx context.Context, scheduleID uuid.UUID) ([]GetContainersForScheduleRow, error) {
	ctx = context.WithValue(ctx, "pggen_query_name", "GetContainersForSchedule")
	rows, err := q.conn.Query(ctx, getContainersForScheduleSQL, scheduleID)
	if err != nil {
		return nil, fmt.Errorf("query GetContainersForSchedule: %w", err)
	}
	defer rows.Close()
	items := []GetContainersForScheduleRow{}
	for rows.Next() {
		var item GetContainersForScheduleRow
		if err := rows.Scan(&item.ID, &item.CreatedAt, &item.UpdatedAt, &item.Name, &item.ContainerImage, &item.Env, &item.Privileged, &item.NetworkMode, &item.Ports, &item.BindDev, &item.BindProc, &item.BindSys, &item.BindShm, &item.BindCgroup, &item.BindDockerSocket, &item.BindBoot, &item.Command, &item.Entrypoint, &item.ScheduleID); err != nil {
			return nil, fmt.Errorf("scan GetContainersForSchedule row: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("close GetContainersForSchedule rows: %w", err)
	}
	return items, err
}

// GetContainersForScheduleBatch implements Querier.GetContainersForScheduleBatch.
func (q *DBQuerier) GetContainersForScheduleBatch(batch genericBatch, scheduleID uuid.UUID) {
	batch.Queue(getContainersForScheduleSQL, scheduleID)
}

// GetContainersForScheduleScan implements Querier.GetContainersForScheduleScan.
func (q *DBQuerier) GetContainersForScheduleScan(results pgx.BatchResults) ([]GetContainersForScheduleRow, error) {
	rows, err := results.Query()
	if err != nil {
		return nil, fmt.Errorf("query GetContainersForScheduleBatch: %w", err)
	}
	defer rows.Close()
	items := []GetContainersForScheduleRow{}
	for rows.Next() {
		var item GetContainersForScheduleRow
		if err := rows.Scan(&item.ID, &item.CreatedAt, &item.UpdatedAt, &item.Name, &item.ContainerImage, &item.Env, &item.Privileged, &item.NetworkMode, &item.Ports, &item.BindDev, &item.BindProc, &item.BindSys, &item.BindShm, &item.BindCgroup, &item.BindDockerSocket, &item.BindBoot, &item.Command, &item.Entrypoint, &item.ScheduleID); err != nil {
			return nil, fmt.Errorf("scan GetContainersForScheduleBatch row: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("close GetContainersForScheduleBatch rows: %w", err)
	}
	return items, err
}

const getDeviceByNameSQL = `SELECT d.*
FROM device AS d
WHERE d.name = $1;`

type GetDeviceByNameRow struct {
	ID        uuid.UUID  `json:"id"`
	Name      *string    `json:"name"`
	CreatedAt *time.Time `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`
	FleetID   uuid.UUID  `json:"fleet_id"`
}

// GetDeviceByName implements Querier.GetDeviceByName.
func (q *DBQuerier) GetDeviceByName(ctx context.Context, name *string) (GetDeviceByNameRow, error) {
	ctx = context.WithValue(ctx, "pggen_query_name", "GetDeviceByName")
	row := q.conn.QueryRow(ctx, getDeviceByNameSQL, name)
	var item GetDeviceByNameRow
	if err := row.Scan(&item.ID, &item.Name, &item.CreatedAt, &item.UpdatedAt, &item.FleetID); err != nil {
		return item, fmt.Errorf("query GetDeviceByName: %w", err)
	}
	return item, nil
}

// GetDeviceByNameBatch implements Querier.GetDeviceByNameBatch.
func (q *DBQuerier) GetDeviceByNameBatch(batch genericBatch, name *string) {
	batch.Queue(getDeviceByNameSQL, name)
}

// GetDeviceByNameScan implements Querier.GetDeviceByNameScan.
func (q *DBQuerier) GetDeviceByNameScan(results pgx.BatchResults) (GetDeviceByNameRow, error) {
	row := results.QueryRow()
	var item GetDeviceByNameRow
	if err := row.Scan(&item.ID, &item.Name, &item.CreatedAt, &item.UpdatedAt, &item.FleetID); err != nil {
		return item, fmt.Errorf("scan GetDeviceByNameBatch row: %w", err)
	}
	return item, nil
}

// textPreferrer wraps a pgtype.ValueTranscoder and sets the preferred encoding
// format to text instead binary (the default). pggen uses the text format
// when the OID is unknownOID because the binary format requires the OID.
// Typically occurs if the results from QueryAllDataTypes aren't passed to
// NewQuerierConfig.
type textPreferrer struct {
	pgtype.ValueTranscoder
	typeName string
}

// PreferredParamFormat implements pgtype.ParamFormatPreferrer.
func (t textPreferrer) PreferredParamFormat() int16 { return pgtype.TextFormatCode }

func (t textPreferrer) NewTypeValue() pgtype.Value {
	return textPreferrer{ValueTranscoder: pgtype.NewValue(t.ValueTranscoder).(pgtype.ValueTranscoder), typeName: t.typeName}
}

func (t textPreferrer) TypeName() string {
	return t.typeName
}

// unknownOID means we don't know the OID for a type. This is okay for decoding
// because pgx call DecodeText or DecodeBinary without requiring the OID. For
// encoding parameters, pggen uses textPreferrer if the OID is unknown.
const unknownOID = 0
