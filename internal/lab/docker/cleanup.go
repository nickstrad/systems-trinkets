package docker

import (
	"context"
	"errors"
	"fmt"

	"github.com/moby/moby/client"
)

// Leftovers names the containers, volumes and networks that carry one
// lesson's label.
type Leftovers struct {
	Containers, Volumes, Networks []string
}

// Empty reports whether nothing is left.
func (l Leftovers) Empty() bool {
	return len(l.Containers)+len(l.Volumes)+len(l.Networks) == 0
}

// ownedBy re-checks on the client side what the server-side filter selected:
// the object must carry both harness labels with this exact lesson. A filter
// bug, an old engine or a typo can then never widen a removal.
func ownedBy(labels map[string]string, lesson string) bool {
	return labels[LabelHarness] == "1" && labels[LabelLesson] == lesson
}

func lessonFilter(lesson string) client.Filters {
	return client.Filters{}.Add("label", LabelHarness+"=1", LabelLesson+"="+lesson)
}

// validLesson guards every list-and-remove call: an empty or odd lesson must
// never reach a label filter.
func validLesson(lesson string) error {
	if !lessonRE.MatchString(lesson) {
		return fmt.Errorf("docker: lesson %q must match %s", lesson, lessonRE)
	}
	return nil
}

// containerName is a container's name without the API's leading slash.
func containerName(names []string, id string) string {
	if len(names) > 0 && len(names[0]) > 1 {
		return names[0][1:]
	}
	return id
}

// ListLesson returns everything that carries the lesson's label: containers (any
// state), volumes and networks, by name.
func ListLesson(ctx context.Context, cli *client.Client, lesson string) (Leftovers, error) {
	if err := validLesson(lesson); err != nil {
		return Leftovers{}, err
	}
	var left Leftovers
	cs, err := cli.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: lessonFilter(lesson)})
	if err != nil {
		return Leftovers{}, fmt.Errorf("docker: list containers of %s: %w", lesson, err)
	}
	for _, c := range cs.Items {
		if ownedBy(c.Labels, lesson) {
			left.Containers = append(left.Containers, containerName(c.Names, c.ID))
		}
	}
	vs, err := cli.VolumeList(ctx, client.VolumeListOptions{Filters: lessonFilter(lesson)})
	if err != nil {
		return Leftovers{}, fmt.Errorf("docker: list volumes of %s: %w", lesson, err)
	}
	for _, v := range vs.Items {
		if ownedBy(v.Labels, lesson) {
			left.Volumes = append(left.Volumes, v.Name)
		}
	}
	ns, err := cli.NetworkList(ctx, client.NetworkListOptions{Filters: lessonFilter(lesson)})
	if err != nil {
		return Leftovers{}, fmt.Errorf("docker: list networks of %s: %w", lesson, err)
	}
	for _, n := range ns.Items {
		if ownedBy(n.Labels, lesson) {
			left.Networks = append(left.Networks, n.Name)
		}
	}
	return left, nil
}

// CheckNoLeak is the no-leak invariant: it returns one violation per
// container, volume or network still carrying the lesson's label. Call it
// after every handle was removed, or after Sweep.
func CheckNoLeak(ctx context.Context, cli *client.Client, lesson string) ([]string, error) {
	left, err := ListLesson(ctx, cli, lesson)
	if err != nil {
		return nil, err
	}
	return noLeak(lesson, left), nil
}

// Sweep force-removes the containers (running ones included, with their
// anonymous volumes), then the volumes, then the networks that carry the
// lesson's label, and nothing else: the server-side label filter selects and
// ownedBy re-checks each object. It is the net for crashed runs; Run and
// handles remove their own containers. It returns what it removed. An object
// that vanished on its own meanwhile is not an error: a failed removal counts
// only if the object is still listed afterwards.
func Sweep(ctx context.Context, cli *client.Client, lesson string) (Leftovers, error) {
	left, err := ListLesson(ctx, cli, lesson)
	if err != nil {
		return Leftovers{}, err
	}
	var removed Leftovers
	failed := map[string]error{}
	try := func(kind, name string, err error, done *[]string) {
		if err != nil {
			failed[kind+" "+name] = err
			return
		}
		*done = append(*done, name)
	}
	for _, name := range left.Containers {
		_, err := cli.ContainerRemove(ctx, name, client.ContainerRemoveOptions{Force: true, RemoveVolumes: true})
		try("container", name, err, &removed.Containers)
	}
	for _, name := range left.Volumes {
		_, err := cli.VolumeRemove(ctx, name, client.VolumeRemoveOptions{Force: true})
		try("volume", name, err, &removed.Volumes)
	}
	for _, name := range left.Networks {
		_, err := cli.NetworkRemove(ctx, name, client.NetworkRemoveOptions{})
		try("network", name, err, &removed.Networks)
	}
	if len(failed) == 0 {
		return removed, nil
	}
	still, err := ListLesson(ctx, cli, lesson)
	if err != nil {
		return removed, fmt.Errorf("docker: sweep %s: %w", lesson, err)
	}
	var errs []error
	for kind, names := range map[string][]string{"container": still.Containers, "volume": still.Volumes, "network": still.Networks} {
		for _, name := range names {
			if err := failed[kind+" "+name]; err != nil {
				errs = append(errs, fmt.Errorf("remove %s %s: %w", kind, name, err))
			}
		}
	}
	if len(errs) > 0 {
		return removed, fmt.Errorf("docker: sweep %s: %w", lesson, errors.Join(errs...))
	}
	return removed, nil
}

// containerGone reports whether the engine no longer lists a container with
// this ID. The client marks a 404 only for errors.Is against containerd's
// errdefs, which this package does not import, so absence is asked directly.
func containerGone(ctx context.Context, cli *client.Client, id string) bool {
	res, err := cli.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: client.Filters{}.Add("id", id)})
	return err == nil && len(res.Items) == 0
}

// CreateVolume creates a labelled named volume trinkets-<lesson>-<role>-<random>
// and returns its name, for a Mount{Type: MountVolume, Source: name}. A fresh
// volume copies the ownership of the image directory it is first mounted on
// (an OwnedDir); Sweep removes it.
func CreateVolume(ctx context.Context, cli *client.Client, lesson, role string) (string, error) {
	if err := validLesson(lesson); err != nil {
		return "", err
	}
	if !lessonRE.MatchString(role) {
		return "", fmt.Errorf("docker: volume role %q must match %s", role, lessonRE)
	}
	name := ContainerName(lesson, role, NameSuffix())
	if _, err := cli.VolumeCreate(ctx, client.VolumeCreateOptions{Name: name, Labels: Labels(lesson)}); err != nil {
		return "", fmt.Errorf("docker: create volume %s: %w", name, err)
	}
	return name, nil
}
