package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/cj123/go-ipsw"
	"github.com/cj123/go-ipsw/api"
)

var (
	identifier, buildid string
	client              = api.NewIPSWClient("https://api.ipsw.me/v4", nil)
)

func init() {
	flag.StringVar(&identifier, "i", "", "the identifier (e.g. iPhone4,1) you want to download")
	flag.StringVar(&buildid, "b", "", "the buildid (e.g. 8C148) you want to download")
	flag.Parse()
}

func main() {
	if identifier == "" || buildid == "" {
		log.Fatal("Invalid identifier/buildid specified.")
	}

	ipswFile, err := ipsw.NewIPSWWithIdentifierBuild(client, identifier, buildid)

	if err != nil {
		log.Fatalf("Could not initialise IPSW, err: %s", err)
	}

	buildManifest, err := ipswFile.BuildManifest()

	if err != nil {
		log.Fatalf("Unable to get BuildManifest, err: %s", err)
	}

	productIndex := -1

	for index, productType := range buildManifest.SupportedProductTypes {
		if productType == identifier {
			productIndex = index
		}
	}

	if productIndex == -1 {
		log.Fatalf("The IPSW specified does not support this device (%s)", identifier)
	}

	saveDir := "tmp"

	err = os.MkdirAll(saveDir, 0700)

	if err != nil {
		log.Fatalf("Could not create temp dir: %s, err: %s", saveDir, err)
	}

	requiredFiles := buildManifest.BuildIdentities[productIndex].Manifest
	requiredFiles["BuildManifest"] = ipsw.Manifest{Info: ipsw.ManifestInfo{Path: ipsw.BuildManifestFilename}}
	requiredFiles["Restore"] = ipsw.Manifest{Info: ipsw.ManifestInfo{Path: ipsw.RestoreFilename}}

	for name, file := range requiredFiles {
		if file.Info.Path == "" {
			continue
		}

		log.Printf("Downloading file: %s to %s", name, file.Info.Path)

		downloadPath := filepath.Join(saveDir, file.Info.Path)

		err := os.MkdirAll(filepath.Dir(downloadPath), 0700)

		if err != nil {
			log.Fatalf("Unable to create download path: %s, err: %s", downloadPath, err)
		}

		f, err := os.Create(downloadPath)

		if err != nil {
			log.Fatalf("Unable to create file: %s, err: %s", downloadPath, err)
		}

		err = ipsw.DownloadFile(ipswFile.Resource, file.Info.Path, f)

		if err != nil {
			log.Fatalf("Unable to download file %s, err: %s", file.Info.Path, err)
		}
	}

	log.Println("Building IPSW file...")

	cwd, err := os.Getwd()

	if err != nil {
		log.Fatalf("Unable to get current directory: %s", err)
	}

	err = os.Chdir(saveDir)

	if err != nil {
		log.Fatalf("Unable to create zip: %s", err)
	}

	err = archive(".", fmt.Sprintf(filepath.Join(cwd, "%s_%s_%s_Restore_Lite.ipsw"), identifier, buildManifest.ProductVersion, buildManifest.ProductBuildVersion))

	if err != nil {
		log.Fatalf("Error creating IPSW file: %s", err)
	}

	log.Println("Done! Happy restoring :-)")
}

func archive(source, target string) error {
	zipfile, err := os.Create(target)

	if err != nil {
		return err
	}

	defer zipfile.Close()

	archive := zip.NewWriter(zipfile)

	defer archive.Close()

	info, err := os.Stat(source)

	if err != nil {
		return nil
	}

	var baseDir string
	if info.IsDir() {
		baseDir = filepath.Base(source)
	}

	filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}

		if baseDir != "" {
			header.Name = filepath.Join(baseDir, strings.TrimPrefix(path, source))
		}

		if info.IsDir() {
			header.Name += "/"
		} else {
			header.Method = zip.Deflate
		}

		writer, err := archive.CreateHeader(header)

		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		file, err := os.Open(path)

		if err != nil {
			return err
		}

		defer file.Close()

		_, err = io.Copy(writer, file)

		return err
	})

	return err
}
