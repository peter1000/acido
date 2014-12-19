package acirenderer

import (
	"archive/tar"
	"container/list"
	"fmt"
	"os"
	"path/filepath"

	ptar "github.com/sgotti/acido/pkg/tar"

	"github.com/appc/spec/schema"
	"github.com/appc/spec/schema/types"
	"github.com/coreos/rocket/cas"
)

// And Image contains the ImageManifest, the Hash and the Level in the dependency tree of this image
type Image struct {
	im    *schema.ImageManifest
	key   string
	level uint16
}

// An ordered slice made of Image. Represent a flatten dependency tree.
// The upper Image should be the first with a level of 0.
// For example if A is the upper images and has two deps (in order B and C). And C has one dep (D):
// The list (reperting the name and excluding im and Hash) should be:
// [{A, Level: 0}, {C, Level:1}, {D, Level: 2}, {B, Level: 1}]
type Images []Image

// Returns the ImageManifest and the Hash of the requested dependency
// This is a fake function that should be replaced by a real image discovery
// and dependency matching
func fakeDepDiscovery(dep types.Dependency, ds *cas.Store) (*schema.ImageManifest, string, error) {
	//if hash.Empty() {
	//	return nil, nil, fmt.Errorf("TODO. Needed dependency hash\n")
	//}
	return ds.GetAci(dep.App, dep.Labels, dep.ImageID)
}

// Returns an ordered list of Image type to be rendered
func CreateDepList(name types.ACName, labels types.Labels, ds *cas.Store) (Images, error) {
	im, key, err := ds.GetAci(name, labels, types.Hash{})
	if err != nil {
		return nil, err
	}
	imgsl := list.New()
	img := Image{im: im, key: key, level: 0}
	imgsl.PushFront(img)
	fmt.Printf("img im: %+v\n", img.im)

	// Create a flatten dependency tree. Use a LinkedList to be able to insert elements in the list while working on it.
	for el := imgsl.Front(); el != nil; el = el.Next() {
		img := el.Value.(Image)
		dependencies := img.im.Dependencies
		for _, d := range dependencies {
			im, key, err := fakeDepDiscovery(d, ds)
			if err != nil {
				return nil, err
			}
			depimg := Image{im: im, key: key, level: img.level + 1}
			fmt.Printf("depimg: %+v\n", depimg)
			imgsl.InsertAfter(depimg, el)
		}
	}

	imgs := Images{}
	for el := imgsl.Front(); el != nil; el = el.Next() {
		imgs = append(imgs, el.Value.(Image))
	}
	return imgs, nil
}

// Given an image hash already available in the store (ds), build its dependency list and render it inside dir
func RenderImage(name types.ACName, labels types.Labels, dir string, ds *cas.Store) error {
	imgs, err := CreateDepList(name, labels, ds)
	if err != nil {
		return err
	}

	fmt.Printf("imgs: %v\n", imgs)

	if len(imgs) == 0 {
		return fmt.Errorf("Image list empty")
	}

	// This implementation needs to start from the end of the tree.
	end := len(imgs) - 1
	prevlevel := imgs[end].level
	for i := end; i >= 0; i-- {
		img := imgs[i]

		err = renderImage(img, dir, ds, prevlevel)
		if err != nil {
			return err
		}
		if img.level < prevlevel {
			prevlevel = img.level
		}
	}
	return nil
}

func renderImage(img Image, dir string, ds *cas.Store, prevlevel uint16) error {
	rs, err := ds.ReadStream(img.key)
	if err != nil {
		return err
	}
	defer rs.Close()
	if err := ptar.ExtractTar(tar.NewReader(rs), dir, true, pwlToMap(img.im.PathWhitelist)); err != nil {
		return fmt.Errorf("error extracting ACI: %v", err)
	}
	// If the image is an a previous level remove files not in
	// PathWhitelist (if PathWhitelist isn't empty)
	// Directories are handled after file removal and all empty directories
	// not in the pathWhiteList will be removed
	if img.level < prevlevel {
		if len(img.im.PathWhitelist) == 0 {
			return nil
		}
		m := pwlToMap(img.im.PathWhitelist)
		rootfs := filepath.Join(dir, "rootfs/")
		err = filepath.Walk(rootfs, func(path string, info os.FileInfo, err error) error {
			if info.IsDir() {
				return nil
			}

			relpath, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}
			if _, ok := m[relpath]; !ok {
				err := os.Remove(path)
				if err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("build: Error walking rootfs: %v", err)
		}

		removeEmptyDirs(dir, rootfs, m)
	}
	return nil
}

func removeEmptyDirs(base string, dir string, pathWhitelistMap map[string]uint8) error {
	dirs, err := getDirectories(dir)
	if err != nil {
		return err
	}
	for _, dir := range dirs {
		removeEmptyDirs(base, dir, pathWhitelistMap)
	}
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	names, err := f.Readdirnames(-1)
	f.Close()
	if err != nil {
		return err
	}
	if len(names) == 0 {
		relpath, err := filepath.Rel(base, dir)
		if err != nil {
			return err
		}
		if _, ok := pathWhitelistMap[relpath]; !ok {
			err := os.Remove(dir)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func getDirectories(dir string) ([]string, error) {

	f, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	infos, err := f.Readdir(-1)
	f.Close()
	if err != nil {
		return nil, err
	}

	dirs := []string{}
	for _, info := range infos {
		if info.IsDir() {
			dirs = append(dirs, filepath.Join(dir, info.Name()))
		}
	}
	return dirs, nil
}

// Convert pathWhiteList slice to a map for faster search
// Also change path to be relative to "/" so it can easyly used without the
// calling function calling filepath.Join("/", ...)
func pwlToMap(pwl []string) map[string]uint8 {
	m := make(ptar.PathWhitelistMap, len(pwl))
	for _, v := range pwl {
		relpath := filepath.Join("rootfs/", v)
		m[relpath] = 1
	}
	return m
}
