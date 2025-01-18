package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"reflect"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"

	"github.com/uinta-labs/pando/models"
)

type genericConn interface {
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
}

var spanData = map[string]interface{}{
	"system": "postgresql",
}

type genericBatch interface {
	// Queue queues a query to batch b. query can be an SQL query or the name of a
	// prepared statement. See Queue on *pgx.Batch.
	Queue(query string, arguments ...interface{})
}

type sentryWrappedQuerier struct {
	pool *pgxpool.Pool
}

func (s *sentryWrappedQuerier) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	operationName, _ := ctx.Value("pggen_query_name").(string)
	span := sentry.StartSpan(ctx, "db.query")
	defer span.Finish()
	span.Data = spanData
	span.Name = operationName
	span.Description = sql

	rows, err := s.pool.Query(span.Context(), sql, args...)
	if err != nil {
		sentry.CaptureException(err)
	}
	return rows, err
}

func (s *sentryWrappedQuerier) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	operationName, _ := ctx.Value("pggen_query_name").(string)
	span := sentry.StartSpan(ctx, "db.query")
	defer span.Finish()
	span.Data = spanData
	span.Name = operationName
	span.Description = sql

	r := s.pool.QueryRow(span.Context(), sql, args...)
	return r
}

func (s *sentryWrappedQuerier) Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error) {
	operationName, _ := ctx.Value("pggen_query_name").(string)
	span := sentry.StartSpan(ctx, "db.query")
	defer span.Finish()
	span.Data = spanData
	span.Name = operationName
	span.Description = sql

	tag, err := s.pool.Exec(span.Context(), sql, arguments...)
	if err != nil {
		sentry.CaptureException(err)
	}
	return tag, err
}

var _ genericConn = &sentryWrappedQuerier{}

type DB struct {
	Pool *pgxpool.Pool
	Q    models.Querier
}

type logger struct{}

func capStringLen(s string, maxLen int) string {
	if len(s) > maxLen {
		return fmt.Sprintf("%s...", s[:maxLen])
	}
	return s
}

func unwrapPointerOrNil(v interface{}) interface{} {
	if v == nil {
		return nil
	}
	if reflect.ValueOf(v).Kind() == reflect.Ptr {
		if reflect.ValueOf(v).IsNil() {
			return nil
		}
		return unwrapPointerOrNil(reflect.ValueOf(v).Elem().Interface())
	}
	return v
}

func interfacesToStrings(interfaces []interface{}) []string {
	strs := make([]string, len(interfaces))
	for i, v := range interfaces {
		if reflect.ValueOf(v).Kind() == reflect.Ptr {
			strs[i] = capStringLen(fmt.Sprintf("%v", unwrapPointerOrNil(v)), 300)
		} else {
			strs[i] = capStringLen(fmt.Sprintf("%v", v), 300)
		}
	}
	return strs
}

func formatQueryData(data map[string]interface{}) string {
	var sql, args string
	sqlValue, ok := data["sql"]
	if ok {
		sql, _ = sqlValue.(string)
	}
	argsValue, ok := data["args"]
	if ok {
		if reflect.ValueOf(argsValue).Kind() == reflect.Slice {
			args = fmt.Sprintf("%v", interfacesToStrings(argsValue.([]interface{})))
		} else if reflect.ValueOf(argsValue).Kind() == reflect.Ptr {
			args = capStringLen(fmt.Sprintf("%v", unwrapPointerOrNil(argsValue)), 300)
		} else {
			args = capStringLen(fmt.Sprintf("%v", argsValue), 300)
		}
	}
	var runTime string
	var rowCount int
	var pid uint32

	if runTimeValue, ok := data["time"]; ok {
		runTimeDuration, _ := runTimeValue.(time.Duration)
		runTime = runTimeDuration.String()
	}
	if rowCountValue, ok := data["rowCount"]; ok {
		rowCount, _ = rowCountValue.(int)
	}
	if pidValue, ok := data["pid"]; ok {
		pid, _ = pidValue.(uint32)
	}
	return fmt.Sprintf("sql[ pid=%d / %s / %d rows ]( %s ): %s", pid, runTime, rowCount, args, capStringLen(sql, 300))
}

func logQueryData(kind string, data map[string]interface{}) string {
	switch kind {
	case "Query":
		if data != nil {
			return formatQueryData(data)
		}
	case "Exec":
		if data != nil {
			return formatQueryData(data)
		}
	default:
	}
	return capStringLen(fmt.Sprintf("%v", data), 100)
}

func (l *logger) Log(ctx context.Context, level pgx.LogLevel, msg string, data map[string]interface{}) {
	if data != nil {
		dataStr := logQueryData(msg, data)
		log.Printf("%s: %s: %s", level, msg, dataStr)
		return
	}
	log.Printf("%s: %s", level, msg)
}

func statMap(stats *pgxpool.Stat) map[string]interface{} {
	return map[string]interface{}{
		"AcquireCount":            stats.AcquireCount(),
		"AcquireDuration":         stats.AcquireDuration(),
		"AcquiredConns":           stats.AcquiredConns(),
		"CanceledAcquireCount":    stats.CanceledAcquireCount(),
		"ConstructingConns":       stats.ConstructingConns(),
		"EmptyAcquireCount":       stats.EmptyAcquireCount(),
		"IdleConns":               stats.IdleConns(),
		"MaxConns":                stats.MaxConns(),
		"NewConnsCount":           stats.NewConnsCount(),
		"MaxLifetimeDestroyCount": stats.MaxLifetimeDestroyCount(),
		"MaxIdleDestroyCount":     stats.MaxIdleDestroyCount(),
	}
}

func New(ctx context.Context, databaseURL string, runStatsServer bool) (*DB, error) {
	l := &logger{}

	pgxConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	pgxConfig.MaxConns = 15 // TODO: Adjust as needed; provide a env var for this
	pgxConfig.ConnConfig.Logger = l
	pgxConfig.ConnConfig.ConnectTimeout = 5 * time.Second
	pool, err := pgxpool.ConnectConfig(ctx, pgxConfig)
	if err != nil {
		return nil, err
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				pool.Close()
				return
			case <-time.After(5 * time.Minute):
				if err := pool.Ping(ctx); err != nil {
					sentry.CaptureMessage("Error pinging database")
					log.Printf("Error pinging database: %#v", err)
				}
				// print db conn stats
				stats := pool.Stat()
				log.Printf("DB Stats: %v", statMap(stats))
			}
		}
	}()
	if runStatsServer {
		go func() {
			time.Sleep(2 * time.Second)
			log.Printf("Starting postgres/db stat http server on :6065")
			runStatHttpServer(ctx, pool)
		}()
	}

	sentryQuerier := &sentryWrappedQuerier{pool: pool}

	return &DB{
		Pool: pool,
		Q:    models.NewQuerier(sentryQuerier),
	}, nil
}

func runStatHttpServer(ctx context.Context, pool *pgxpool.Pool) {
	httpStatAddress := "0.0.0.0:6065"
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<h1>Postgres DB Stats</h1>
		<p><a href="/stat">/stat</a></p>`)
	})
	http.HandleFunc("/stat", func(w http.ResponseWriter, r *http.Request) {
		stats := pool.Stat()
		asJson, err := json.Marshal(statMap(stats))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "%s", asJson)
	})

	err := http.ListenAndServe(httpStatAddress, nil)
	if err != nil {
		log.Printf("Error starting http server: %v", err)
	}
}

func WaitForDatabase(ctx context.Context, timeoutSeconds int, databaseURL string) error {
	deadlineTime := time.Now().Add(time.Duration(timeoutSeconds) * time.Second)
	fmt.Printf("[DEBUG] Waiting for database at %s\n", databaseURL)
	fmt.Printf("waiting up to %d second(s) for database to be ready", timeoutSeconds)
	for {
		select {
		case <-ctx.Done():
			fmt.Print("\n")
			return ctx.Err()
		default:
		}
		if time.Now().After(deadlineTime) {
			fmt.Print("\n")
			return errors.New("timeout waiting for database")
		}
		fmt.Print(".")
		pool, err := pgxpool.Connect(ctx, databaseURL)
		if err == nil {
			pool.Close()
			fmt.Print("\n")
			return nil
		}
		time.Sleep(1 * time.Second)
	}
}
