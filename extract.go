package main

import (
	"io/ioutil"

	"github.com/appc/spec/discovery"
	"github.com/coreos/fleet/log"
	"github.com/coreos/rocket/cas"
	"github.com/coreos/rocket/pkg/acirenderer"
	"github.com/sgotti/acido/util"
)

var (
	cmdExtract = &Command{
		Name:        "extract",
		Summary:     "Extracts an image already imported in the store (satisfying all its dependencies)",
		Usage:       "IMAGEHASH",
		Description: `IMAGEHASH hash of base image (it must exists in the store or it sould be imported with the \"import\" command.`,
		Run:         runExtract,
	}
)

func init() {
	commands = append(commands, cmdExtract)
}

func runExtract(args []string) (exit int) {
	ds := cas.NewStore(globalFlags.Dir)

	name := args[0]
	app, err := discovery.NewAppFromString(name)
	// TODO temp hack, remove latest label. See appc/spec#86
	if app.Labels["version"] == "latest" {
		delete(app.Labels, "version")
	}

	tmpdir, err := ioutil.TempDir(globalFlags.WorkDir, "")
	if err != nil {
		log.Errorf("error: %v", err)
		return 1
	}
	log.V(1).Infof("tmpdir: %s", tmpdir)
	labels, err := util.MapToLabels(app.Labels)
	if err != nil {
		log.Errorf("error: %v", err)
		return 1
	}
	err = acirenderer.RenderACI(app.Name, labels, tmpdir, ds)
	if err != nil {
		log.Errorf("error: %v", err)
		return 1
	}
	log.Infof("Image extracted to %s", tmpdir)
	return 0
}
