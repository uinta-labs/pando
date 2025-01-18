migrate:
    pnpx prisma migrate dev

fmt:
    gofmt -s -w .
    pnpm run format

db:
    docker compose down || true
    docker compose up -d

gen:
    #!/usr/bin/env bash
    set -euxo pipefail

    # reset db latency to 0
    previous_db_latency=$(just db-latency)
    just throttle-db 0 || true

    just migrate

    pggen gen go \
      --postgres-connection $DATABASE_URL \
      --query-glob 'models/**/queries*.sql' \
      --go-type 'int8=*int' \
      --go-type 'float8=*float64' \
      --go-type 'text=*string' \
      --go-type 'varchar=*string' \
      --go-type '_text=[]string' \
      --go-type '_varchar=[]string' \
      --go-type 'uuid=github.com/google/uuid.UUID' \
      --go-type '_uuid=[]github.com/google/uuid.UUID' \
      --go-type 'timestamp=*time.Time' \
      --go-type 'timestamptz=*time.Time' \
      --go-type 'timestamptzs=*time.Time' \
      --go-type 'jsonb=[]byte'

    buf generate

    go run ./pggen_proto_map -input "./models/*.sql" -module "models" -output ./gen/protos/remote/upd88/com/comconnect/mapper.gen.go

    cp ./gen_convert/_convert.go ./gen/protos/remote/upd88/com/comconnect/convert.gen.go
    goimports -w -local github.com/uinta-labs/ models/*.sql.go
    go build ./gen/... # build to check for errors
    just fmt

    if [ -n "$previous_db_latency" ]; then
      echo "Restoring previous db latency of $previous_db_latency"
      just throttle-db "$previous_db_latency"
    fi

throttle-db rtt="100ms":
    #!/usr/bin/env bash
    set -euo pipefail
    rtt="{{ rtt }}"

    docker compose exec -it db bash -c '(tc -h 2>&1) > /dev/null || apk add iproute2'

    docker compose exec -it db bash -c 'tc qdisc del dev eth0 root || true'

    if [ "$rtt" == "0" ]; then
      exit 0
    fi

    # Throttle the database connection to simulate network latency
    docker compose exec -it db tc qdisc add dev eth0 root netem delay $rtt
    # list the rules
    docker compose exec -it db tc qdisc show dev eth0

db-latency:
    docker compose exec -it db tc qdisc show dev eth0 | grep -oE 'delay [0-9]+ms' | cut -d' ' -f2
