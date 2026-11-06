package natsexperiment

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	tc "github.com/testcontainers/testcontainers-go"
	tcnats "github.com/testcontainers/testcontainers-go/modules/nats"
)

const DefaultImage = "nats@sha256:afae2fdbe2e326c65e1246cb83b9d05d319188734f3bd577ac8072daae78a10f"

type Broker struct {
	Container *tcnats.NATSContainer
	URL       string
	Image     string
}

func StartBroker(ctx context.Context, image, cpus string) (*Broker, error) {
	if !strings.Contains(image, "@sha256:") {
		return nil, fmt.Errorf("NATS image must be pinned by digest")
	}
	port := network.MustParsePort("4222/tcp")
	options := []tc.ContainerCustomizer{
		tc.CustomizeRequestOption(func(req *tc.GenericContainerRequest) error {
			// Override module defaults -DV/-js and unneeded routing/monitoring ports.
			req.Cmd = []string{"-config", "/etc/nats.conf"}
			req.ExposedPorts = []string{"4222/tcp"}
			req.Env = map[string]string{"GOMAXPROCS": "2"}
			req.Files = append(req.Files, tc.ContainerFile{Reader: strings.NewReader("server_name: jsontrans-message-experiment\nport: 4222\nmax_payload: 2097152\njetstream: false\ndebug: false\ntrace: false\n"), ContainerFilePath: "/etc/nats.conf", FileMode: 0644})
			return nil
		}),
		tc.WithConfigModifier(func(c *container.Config) { c.ExposedPorts = network.PortSet{port: struct{}{}} }),
		tc.WithHostConfigModifier(func(h *container.HostConfig) {
			h.PublishAllPorts = false
			h.CpusetCpus = cpus
			h.Memory = 512 << 20
			h.PortBindings = network.PortMap{port: []network.PortBinding{{HostIP: netip.MustParseAddr("127.0.0.1"), HostPort: ""}}}
		}),
	}
	c, err := tcnats.Run(ctx, image, options...)
	if err != nil {
		if c != nil {
			cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			c.Terminate(cleanup)
		}
		return nil, err
	}
	url, err := c.ConnectionString(ctx)
	if err != nil {
		cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cleanupErr := c.Terminate(cleanup)
		if cleanupErr != nil {
			return nil, fmt.Errorf("%w; cleanup: %v", err, cleanupErr)
		}
		return nil, err
	}
	return &Broker{c, url, image}, nil
}
func (b *Broker) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return b.Container.Terminate(ctx)
}
