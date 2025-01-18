package pkg

import (
	"bufio"
	"context"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/registry"
	"io"
	"log"
	"os"

	"github.com/pkg/errors"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

type Runner struct {
	client *client.Client
}

type LogChannels struct {
	BaseContext   context.Context
	Mixed         chan string
	ChannelClosed chan struct{}
}

func NewLogChannels(ctx context.Context) *LogChannels {
	return &LogChannels{
		BaseContext:   ctx,
		Mixed:         make(chan string),
		ChannelClosed: make(chan struct{}),
	}
}

type registryCredentials struct {
	GHCRUsername string
	GHCRToken    string
}

func NewRegistryCredentials(ghcrUsername string, ghcrToken string) *registryCredentials {
	return &registryCredentials{
		GHCRUsername: ghcrUsername,
		GHCRToken:    ghcrToken,
	}
}

func (r *registryCredentials) GetAuthenticationString() string {
	authConfig := registry.AuthConfig{
		Username: r.GHCRUsername,
		Password: r.GHCRToken,
	}
	resp, err := registry.EncodeAuthConfig(authConfig)
	if err != nil {
		log.Panicln(err)
	}
	return resp
}

func (l *LogChannels) AttachScanner(scanner *bufio.Scanner) {
	go func() {
		for scanner.Scan() {
			select {
			case <-l.BaseContext.Done():
				close(l.Mixed)
				close(l.ChannelClosed)
				return
			case <-l.ChannelClosed:
				return
			default:
				l.Mixed <- scanner.Text()
			}
		}
	}()
}

func (l *LogChannels) Close() {
	select {
	case <-l.BaseContext.Done():
		return
	case l.ChannelClosed <- struct{}{}:
	}
}

func (l *LogChannels) Consumer() <-chan string {
	return l.Mixed
}

func NewRunner(hostSocketLocation string) *Runner {

	var cli *client.Client
	var err error

	cli, err = client.NewClientWithOpts(client.WithHost(hostSocketLocation), client.WithAPIVersionNegotiation())
	if err != nil {
		log.Panicln("failed to create docker client:", err)
	}

	return &Runner{
		client: cli,
	}
}

func (r *Runner) PullImage(ctx context.Context, imageReference string) error {
	reader, err := r.client.ImagePull(ctx, imageReference, image.PullOptions{})
	if err != nil {
		log.Panicf("failed to pull image: %s", err)
	}

	defer reader.Close()
	_, err = io.Copy(os.Stdout, reader)
	return err
}

func (r *Runner) PullImageWithCredentials(ctx context.Context, imageReference string, credentials *registryCredentials) error {
	reader, err := r.client.ImagePull(ctx, imageReference, image.PullOptions{
		RegistryAuth: credentials.GetAuthenticationString(),
	})
	if err != nil {
		log.Printf("failed to pull image: %s", err)
		return err
	}

	defer reader.Close()
	_, err = io.Copy(os.Stdout, reader)
	return err
}

func (r *Runner) FindFirstAvailableImage(ctx context.Context, credentials *registryCredentials, imageReferences []string) (string, error) {
	for _, imageReference := range imageReferences {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if err := r.PullImageWithCredentials(ctx, imageReference, credentials); err != nil {
			log.Printf("failed to pull image %s: %s", imageReference, err)
			continue
		}
		return imageReference, nil
	}
	return "", errors.New("no images available")
}

type AdvancedOptions struct {
	BindMountDockerSocket      bool
	NetworkModeContainer       string
	NetworkModeHost            bool
	DockerEngineSocketOverride string
}

func (r *Runner) RunContainer(ctx context.Context, imageReference string, containerReference string, commands []string, environmentVariables []string, additionalLabels map[string]string, advancedOptions *AdvancedOptions, logs *LogChannels, waitOnContainer bool) (string, error) {
	networkMode := container.NetworkMode("bridge")
	binds := []string{}
	if advancedOptions != nil {
		if advancedOptions.NetworkModeContainer != "" && advancedOptions.NetworkModeHost {
			return "", errors.New("cannot specify both network mode container and network mode host")
		}
		if advancedOptions.NetworkModeContainer != "" {
			networkMode = container.NetworkMode(advancedOptions.NetworkModeContainer)
		}
		if advancedOptions.NetworkModeHost {
			networkMode = container.NetworkMode("host")
		}
		if advancedOptions.BindMountDockerSocket {
			binds = []string{}
			if advancedOptions.DockerEngineSocketOverride != "" {
				binds = append(binds, advancedOptions.DockerEngineSocketOverride+":/var/run/docker.sock")
			} else {
				binds = append(binds, "/var/run/docker.sock:/var/run/docker.sock")
			}
		}
	}

	labels := map[string]string{
		"io.uinta.pando.managed":             "true",
		"io.uinta.pando.container-reference": containerReference,
	}
	for k, v := range additionalLabels {
		labels[k] = v
	}

	resp, err := r.client.ContainerCreate(ctx, &container.Config{
		Image:        imageReference,
		Cmd:          commands,
		Tty:          true,
		AttachStderr: true,
		AttachStdout: true,
		Labels:       labels,
		Env:          environmentVariables,
	}, &container.HostConfig{
		AutoRemove: true,
		ConsoleSize: [2]uint{
			140,
			60,
		},
		NetworkMode: networkMode,
		Binds:       binds,
	}, nil, nil, containerReference)
	if err != nil {
		return "", err
	}

	if err := r.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		log.Panicf("failed to start container: %s", err)
	}

	go func() {
		out, err := r.client.ContainerLogs(ctx, resp.ID, container.LogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Follow:     true,
		})
		if err != nil {
			log.Println("failed to get container logs:", err)
			return
		}
		logs.AttachScanner(bufio.NewScanner(out))
	}()

	if waitOnContainer {
		statusCh, errCh := r.client.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
		select {
		case err := <-errCh:
			if err != nil {
				log.Panicf("failed to wait for container: %s", err)
			}
		case stat := <-statusCh:
			if stat.StatusCode != 0 {
				return resp.ID, errors.New("container exited with non-zero exit code")
			}
		}
	}

	return resp.ID, nil
}

func (r *Runner) ExecCommand(ctx context.Context, containerReference string, command []string, logs *LogChannels) error {
	execConfig := container.ExecOptions{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          command,
	}
	execID, err := r.client.ContainerExecCreate(ctx, containerReference, execConfig)
	if err != nil {
		return err
	}

	resp, err := r.client.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{
		Detach:      false,
		Tty:         true,
		ConsoleSize: &[2]uint{140, 60},
	})
	if err != nil {
		return err
	}
	defer resp.Close()

	logs.AttachScanner(bufio.NewScanner(resp.Reader))

	if err := r.client.ContainerExecStart(ctx, execID.ID, container.ExecStartOptions{
		Detach: false,
		Tty:    true,
		ConsoleSize: &[2]uint{
			140,
			60,
		},
	}); err != nil {
		return err
	}

	return nil
}

// ExecCommandString runs a single command, waiting for it to complete, then returns all stdout/stderr as a string
func (r *Runner) ExecCommandString(ctx context.Context, containerReference string, command []string) (string, error) {
	execConfig := container.ExecOptions{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          command,
	}
	execID, err := r.client.ContainerExecCreate(ctx, containerReference, execConfig)
	if err != nil {
		return "", err
	}

	resp, err := r.client.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{
		Detach:      false,
		Tty:         true,
		ConsoleSize: &[2]uint{140, 60},
	})
	if err != nil {
		return "", err
	}
	defer resp.Close()

	var output string
	scanner := bufio.NewScanner(resp.Reader)
	for scanner.Scan() {
		output += scanner.Text()
	}

	if err := r.client.ContainerExecStart(ctx, execID.ID, container.ExecStartOptions{
		Detach: false,
		Tty:    true,
		ConsoleSize: &[2]uint{
			140,
			60,
		},
	}); err != nil {
		return "", err
	}

	return output, nil
}

func (r *Runner) KillContainer(ctx context.Context, containerReference string) error {
	return r.client.ContainerKill(ctx, containerReference, "SIGTERM")
}

func (r *Runner) ContainerIsRunning(ctx context.Context, containerReference string) (bool, error) {
	c, err := r.client.ContainerInspect(ctx, containerReference)
	if err != nil {
		return false, err
	}
	return c.State.Running, nil
}

func (r *Runner) WaitForContainerToExit(ctx context.Context, containerReference string) error {
	statusCh, errCh := r.client.ContainerWait(ctx, containerReference, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			return err
		}
	case <-statusCh:
	case <-ctx.Done():
		return nil
	}
	return nil
}

func (r *Runner) ListContainersMatchingLabel(ctx context.Context, label string, value string) ([]types.Container, error) {
	return r.client.ContainerList(ctx, container.ListOptions{
		Filters: filters.NewArgs(filters.Arg("label", label+"="+value)),
	})
}
