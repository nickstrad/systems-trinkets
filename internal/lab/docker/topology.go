package docker

import (
	"context"
	"errors"

	"github.com/moby/moby/client"
)

// SocketDir is where both SocketVolume mounts put the shared socket
// directory inside the broker and the workers.
const SocketDir = "/sock"

// SocketVolume creates a labelled named volume and prepares it so a broker
// running as brokerUID:socketGID can bind a Unix socket in it: a one-shot
// init container (root, CapDrop ALL, CapAdd CHOWN and FOWNER, no network,
// read-only root) sets the directory to brokerUID:socketGID mode 2750.
// broker is the read-write Mount for the broker; worker is the Mount for
// workers, who reach the socket through supplementary group socketGID.
// Both target SocketDir. Sweep removes the volume.
//
// Stub: the interface is fixed so H5 and H6 can be built in parallel; H5
// replaces the body.
func SocketVolume(ctx context.Context, cli *client.Client, lesson string, brokerUID, socketGID int) (broker, worker Mount, err error) {
	return Mount{}, Mount{}, errors.New("docker: SocketVolume is not implemented yet (H5)")
}
