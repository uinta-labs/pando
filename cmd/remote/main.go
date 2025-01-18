package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"connectrpc.com/connect"

	"github.com/uinta-labs/pando/gen/protos/remote/upd88/com"
	"github.com/uinta-labs/pando/gen/protos/remote/upd88/com/comconnect"
	"github.com/uinta-labs/pando/pkg"
)

const (
	DefaultBaseUrl     = "https://graphene.fluffy-broadnose.ts.net"
	DockerEngineSocket = "/var/run/balena-engine.sock"
)

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func applySchedule(ctx context.Context, client comconnect.RemoteServiceClient, runner *pkg.Runner, schedule *com.Schedule) error {

	// prune old containers i.e. containers that are not in the schedule
	existingContainers, err := runner.ListContainersMatchingLabel(ctx, "io.uinta.pando.managed", "true")
	if err != nil {
		log.Printf("Error listing containers: %v", err)
		return err
	}

	currentlyRunningContainers := map[string]bool{}
	for _, container := range existingContainers {
		found := false
		for _, task := range schedule.Containers {
			if container.Labels["io.uinta.pando.task-id"] == task.Id {
				found = true
				currentlyRunningContainers[task.Id] = true
				continue
			}
		}
		if !found {
			log.Printf("Removing container %s", container.ID)
			err := runner.KillContainer(ctx, container.ID)
			if err != nil {
				log.Printf("Error removing container: %v", err)
			}
		}
	}

	log.Printf("Running schedule: %s", schedule.Id)

	for _, task := range schedule.Containers {
		if currentlyRunningContainers[task.Id] {
			log.Printf("Task %s already running", task.Id)
			continue
		}
		startImageCtx := context.WithValue(ctx, "task", task)
		log.Printf("Running task: %s", task.Name)
		err := runner.PullImage(startImageCtx, task.ContainerImage)
		if err != nil {
			log.Printf("Error pulling image: %v", err)
			continue
		}

		var containerID string
		logChannels := pkg.NewLogChannels(ctx)
		go func() {
			// for now, leak a goroutine and just log the output
			for {
				select {
				case <-ctx.Done():
					return
				case <-logChannels.ChannelClosed:
					return
				case line := <-logChannels.Mixed:
					log.Printf("Container %s: %s", containerID, line)
				}
			}
		}()
		environmentVariables := []string{}
		for k, v := range task.Env {
			environmentVariables = append(environmentVariables, fmt.Sprintf("%s=%s", k, v))
		}
		commandLine := []string{}
		if task.Command != "" {
			commandLine = append(commandLine, task.Command)
		}

		labels := map[string]string{
			"io.uinta.pando.task-id":     task.Id,
			"io.uinta.pando.task-name":   task.Name,
			"io.uinta.pando-schedule-id": schedule.Id,
		}

		containerID, err = runner.RunContainer(startImageCtx, task.ContainerImage, task.Id, commandLine, environmentVariables, labels, &pkg.AdvancedOptions{
			BindMountDockerSocket: task.BindDockerSocket,
			//NetworkModeContainer:  "",
			NetworkModeHost:            task.NetworkMode == com.Container_HOST,
			DockerEngineSocketOverride: getEnv("DOCKER_HOST", DockerEngineSocket),
		}, logChannels, false)
		if err != nil {
			log.Printf("Error running container: %v", err)
			continue
		}

		log.Printf("Container %s(%s) started", task.Id, containerID)
	}

	return nil
}

func runSchedulerTick(ctx context.Context, client comconnect.RemoteServiceClient, runner *pkg.Runner) {
	log.Println("Running scheduler")
	hostname, err := os.Hostname()
	if err != nil {
		log.Printf("Error: %v", err)
	}
	schedule, err := client.GetSchedule(ctx, &connect.Request[com.GetScheduleRequest]{
		Msg: &com.GetScheduleRequest{
			DeviceId: hostname,
		},
	})
	if err != nil {
		log.Printf("Error: %v", err)
	}

	if schedule != nil && schedule.Msg != nil && schedule.Msg.Schedule != nil {
		err = applySchedule(ctx, client, runner, schedule.Msg.Schedule)
		if err != nil {
			log.Printf("Error applying schedule: %v", err)
			return
		}
	} else {
		log.Println("Received empty schedule")
	}
}

func runScheduler(ctx context.Context, client comconnect.RemoteServiceClient, runner *pkg.Runner) {
	runSchedulerTick(ctx, client, runner)
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(15 * time.Second):
			runSchedulerTick(ctx, client, runner)
		}
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	time.Local = time.UTC

	dockerClient := pkg.NewRunner(getEnv("DOCKER_HOST", DockerEngineSocket))

	httpClient := &http.Client{}

	apiURL := getEnv("API_URL", DefaultBaseUrl)
	log.Printf("API_URL: %s", apiURL)

	client := comconnect.NewRemoteServiceClient(httpClient, apiURL, connect.WithClientOptions(connect.WithSendGzip()))

	go runScheduler(ctx, client, dockerClient)

	<-ctx.Done()
	log.Println("Shutting down")
}
