package main

import (
	"archive/zip"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cj123/ranger"
	"github.com/dhowett/go-plist"
)

const defaultBufferSize = 128 * 1024

var (
	identifier, downloadURL string
)

type BuildManifest struct {
	BuildIdentities       []BuildIdentity
	SupportedProductTypes []string
	ProductVersion        string
	ProductBuildVersion   string
}

type BuildIdentity struct {
	Info     BuildIdentityInfo
	Manifest BuildIdentityManifest
}

type BuildIdentityInfo struct {
	BuildNumber string
	BuildTrain  string
	DeviceClass string
}

type BuildIdentityManifest map[string]Manifest

type Manifest struct {
	Info ManifestInfo
}

type ManifestInfo struct {
	Path string
}

func init() {
	flag.StringVar(&identifier, "i", "", "the identifier (e.g. iPhone4,1) you want to download")
	flag.StringVar(&downloadURL, "u", "", "the URL of the IPSW")
	flag.Parse()
}

func main() {
	if identifier == "" || downloadURL == "" {
		log.Fatal("Invalid identifier/url specified.")
	}

	buildManifestData := new(bytes.Buffer)

	// first off, grab the build manifest
	err := downloadFile(downloadURL, "BuildManifest.plist", buildManifestData)

	if err != nil {
		log.Fatalf("Unable to fetch BuildManifest, err: %s", err)
	}

	var buildManifest BuildManifest

	_, err = plist.Unmarshal(buildManifestData.Bytes(), &buildManifest)

	if err != nil {
		log.Fatalf("Unable to read BuildManifest, err: %s", err)
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
	requiredFiles["BuildManifest"] = Manifest{ManifestInfo{Path: "BuildManifest.plist"}}
	requiredFiles["Restore"] = Manifest{ManifestInfo{Path: "Restore.plist"}}

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

		err = downloadFile(downloadURL, file.Info.Path, f)

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

func downloadFile(resource, file string, w io.Writer) error {
	u, err := url.Parse(resource)

	if err != nil {
		return err
	}

	// retrieve the Restore.plist from the IPSW, possibly need the BuildManifest too?
	reader, err := ranger.NewReader(
		&ranger.HTTPRanger{
			URL: u,
			Client: &http.Client{
				Timeout: 30 * time.Second,
			},
		},
	)

	if err != nil {
		return err
	}

	zipReader, err := zip.NewReader(reader, reader.Length())

	if err != nil {
		return err
	}

	for _, f := range zipReader.File {
		if f.Name == file {
			return bufferedDownload(f, w)
		}
	}

	return errors.New("file not found")
}

func bufferedDownload(file *zip.File, writer io.Writer) error {
	rc, err := file.Open()

	if err != nil {
		return err
	}

	defer rc.Close()

	downloaded := uint64(0)
	filesize := file.UncompressedSize64
	buf := make([]byte, defaultBufferSize)

	for {
		// adjust the size of the buffer to get the exact
		// number of bytes we want to download
		if downloaded+defaultBufferSize > filesize {
			buf = make([]byte, filesize-downloaded)
		}

		if n, err := io.ReadFull(rc, buf); n > 0 {
			writer.Write(buf[:n])
			downloaded += uint64(n)
		} else if err != nil && err != io.EOF {
			return err
		} else {
			break
		}
	}

	return nil
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
