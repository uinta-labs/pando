package main

import (
	"connectrpc.com/connect"
	"context"
	"encoding/json"
	"flag"
	"github.com/google/uuid"
	"github.com/uinta-labs/pando/gen/protos/remote/upd88/com"
	"github.com/uinta-labs/pando/pkg"
	"github.com/uinta-labs/pando/pkg/db"
	"log"
	"net/http"
	"os"
	"time"

	_ "connectrpc.com/connect"
	"connectrpc.com/grpcreflect"
	"github.com/parrotmac/goutil"
	"github.com/pkg/errors"
	"github.com/rs/cors"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/uinta-labs/pando/gen/protos/remote/upd88/com/comconnect"
)

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startTime := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(startTime))
	})
}

type server struct {
	db *db.DB
}

func (s *server) GetSchedule(ctx context.Context, req *connect.Request[com.GetScheduleRequest]) (*connect.Response[com.GetScheduleResponse], error) {
	deviceID := req.Msg.GetDeviceId()
	log.Printf("GetSchedule: %s\n", deviceID)

	deviceUUID, err := uuid.Parse(deviceID)
	if err != nil {
		// try to use device name to lookup before considering failed
		log.Printf("Failed to parse device id: %s\n", err)
		log.Printf("Trying to resolve by name %s\n", deviceID)
		deviceFromName, deviceFromNameErr := s.db.Q.GetDeviceByName(ctx, &deviceID)
		if deviceFromNameErr != nil {
			return nil, errors.Wrap(deviceFromNameErr, "failed to parse device id or resolve by name")
		}
		deviceUUID = deviceFromName.ID
	}

	schedule, err := s.db.Q.GetCurrentScheduleForDevice(ctx, deviceUUID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get schedule")
	}

	scheduleComponents, err := s.db.Q.GetContainersForSchedule(ctx, schedule.ID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get schedule components")
	}

	containers := make([]*com.Container, 0, len(scheduleComponents))
	for _, component := range scheduleComponents {
		env := make(map[string]string, len(component.Env))
		err = json.Unmarshal(component.Env, &env)
		if err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal env")
		}

		var networkMode com.Container_NetworkMode
		if component.NetworkMode != nil {
			switch *component.NetworkMode {
			case "host":
				networkMode = com.Container_HOST
			case "bridge":
				networkMode = com.Container_BRIDGE
			case "none":
				networkMode = com.Container_NONE
			// TODO: Support 'container' network mode (join network of another container)
			// Also have a more clear default/zero value
			default:
				networkMode = com.Container_BRIDGE
			}
		}

		ports := make([]*com.Container_Port, 0, len(component.Ports))
		err = json.Unmarshal(component.Ports, &ports)
		if err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal ports")
		}

		containers = append(containers, &com.Container{
			Id:               component.ID.String(),
			Name:             goutil.UnwrapOr(component.Name, ""),
			ContainerImage:   goutil.UnwrapOr(component.ContainerImage, ""),
			Env:              env,
			Privileged:       component.Privileged,
			NetworkMode:      networkMode,
			Ports:            ports,
			BindDev:          component.BindDev,
			BindProc:         component.BindProc,
			BindSys:          component.BindSys,
			BindShm:          component.BindShm,
			BindCgroup:       component.BindCgroup,
			BindDockerSocket: component.BindDockerSocket,
			BindBoot:         component.BindBoot,
			Command:          goutil.UnwrapOr(component.Command, ""),
			Entrypoint:       goutil.UnwrapOr(component.Entrypoint, ""),
		})
	}

	resp := &com.GetScheduleResponse{
		Schedule: &com.Schedule{
			Id:         schedule.ID.String(),
			Current:    true,
			Containers: containers,
		},
	}

	return &connect.Response[com.GetScheduleResponse]{
		Msg: resp,
	}, nil
}

func (s *server) ReportScheduleState(ctx context.Context, req *connect.Request[com.ReportScheduleStateRequest]) (*connect.Response[com.ReportScheduleStateResponse], error) {
	return nil, nil
}

func main() {
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := pkg.ReadConfig()
	if err != nil {
		log.Panicf("failed to read config: %+v\n", err)
	}

	db, err := db.New(ctx, cfg.DatabaseURL, true)
	if err != nil {
		log.Panicf("failed to connect to database: %+v\n", err)
	}

	srv := &server{
		db: db,
	}

	httpMux := http.NewServeMux()

	reflector := grpcreflect.NewStaticReflector(
		"remote.upd88.com.RemoteService",
	)
	httpMux.Handle(grpcreflect.NewHandlerV1(reflector))
	httpMux.Handle(grpcreflect.NewHandlerV1Alpha(reflector))

	{
		baseURL, connectHandler := comconnect.NewRemoteServiceHandler(srv)
		log.Printf("Binding RemoteService to %s\n", baseURL)
		httpMux.Handle(baseURL, connectHandler)
	}

	corsConfig := cors.New(cors.Options{
		AllowOriginFunc: func(origin string) bool {
			return true
		},
		AllowedOrigins: []string{
			"http://localhost:3000",
		},
		AllowedMethods: []string{
			"GET",
			"PATCH",
			"POST",
			"OPTIONS",
		},
		AllowCredentials: true,
		AllowedHeaders: []string{
			"Authorization",
			"Baggage",
			"Connect-Protocol-Version",
			"Content-Type",
			"Cookie",
			"Origin",
			"Sentry-Trace",
			"User-Agent",
			"Baggage",
			"Sentry-Trace",
		},
		Debug: cfg.Debug,
	})

	withLogging := loggingMiddleware(httpMux)
	withCors := corsConfig.Handler(withLogging)
	httpServer := http.Server{
		Addr:              cfg.Host + ":" + cfg.Port,
		Handler:           h2c.NewHandler(withCors, &http2.Server{}),
		ReadTimeout:       time.Second * 30,
		WriteTimeout:      time.Second * 30,
		IdleTimeout:       time.Second * 60,
		ReadHeaderTimeout: time.Second * 10,
		ErrorLog:          log.New(os.Stderr, "HTTP Server: ", log.LstdFlags),
	}

	go func() {
		log.Println("Starting server on", httpServer.Addr)
		err := httpServer.ListenAndServe()
		if err != nil {
			log.Println("Server error:", err)
		}
	}()

	<-ctx.Done()

	log.Println("Shutting down server...")
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Println("Server shutdown error:", err)
	}
}
