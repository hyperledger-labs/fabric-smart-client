/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package weaver

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	interopCCPath = `https://github.com/hyperledger-labs/weaver-dlt-interoperability/archive/refs/tags/core/network/fabric-interop-cc/contracts/interop/v1.2.2.zip`
)

func packageChaincode() (tmpDir string, cleanup func(), err error) {
	metadataContent := `{"path":"github.com/hyperledger-labs/weaver-dlt-interoperability/core/network/fabric-interop-cc/contracts/interop","type":"golang","label":"interop"}`
	_ = metadataContent
	resp, err := http.Get(interopCCPath)
	if err != nil {
		return "", nil, fmt.Errorf("failed downloading file %s: %v", interopCCPath, err)
	}
	defer resp.Body.Close()

	tmpDir, err = ioutil.TempDir("", "interopCC")
	if err != nil {
		return "", nil, fmt.Errorf("failed creating temp directory: %v", err)
	}

	archivePath := filepath.Join(tmpDir, "interopCC.zip")
	out, err := os.Create(archivePath)
	if err != nil {
		return "", nil, err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return "", nil, fmt.Errorf("failed downloading data from %s: %v", interopCCPath, err)
	}

	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", nil, fmt.Errorf("failed opening zip archive %s: %v", archivePath, err)
	}
	defer archive.Close()

	for _, entry := range archive.File {
		if !strings.Contains(entry.Name, "/core/network/fabric-interop-cc/contracts/interop/") {
			continue
		}

		i := strings.Index(entry.Name, "/core/network/fabric-interop-cc/contracts/interop/")
		if i == -1 {
			continue
		}

		relPath := entry.Name[i:]

		relPath = strings.Replace(relPath, "/core/network/fabric-interop-cc/contracts/interop", "", 1)
		relPath = filepath.Join(tmpDir, "src", relPath)

		if strings.Contains(relPath, "..") {
			return "", nil, fmt.Errorf("illegal path (contains '..')")
		}

		fi := entry.FileInfo()

		if fi.IsDir() {
			err := os.MkdirAll(relPath, 0o755)
			if err != nil {
				return "", nil, fmt.Errorf("failed creating directory %s: %v", relPath, err)
			}
			continue
		}

		f, err := entry.Open()
		if err != nil {
			return "", nil, fmt.Errorf("invalid archive entry %s: %v", entry.Name, err)
		}

		bytes, err := ioutil.ReadAll(f)
		if err != nil {
			return "", nil, fmt.Errorf("failed reading %s: %v", entry.Name, err)
		}

		err = ioutil.WriteFile(relPath, bytes, 0o644)
		if err != nil {
			return "", nil, fmt.Errorf("failed writing to %s: %v", relPath, err)
		}
	} // for all entries in zip file

	srcFolder := filepath.Join(tmpDir, "src")
	codeTarGz := filepath.Join(tmpDir, "code.tar.gz")
	metadataPath := filepath.Join(tmpDir, "metadata.json")

	err = gzipDir(srcFolder, codeTarGz)
	if err != nil {
		return tmpDir, func() {}, err
	}

	err = ioutil.WriteFile(metadataPath, []byte(metadataContent), 0o644)
	if err != nil {
		return
	}

	packageTarGz := filepath.Join(tmpDir, "interop.tar.gz")

	createCodePackage(packageTarGz, codeTarGz, metadataPath)

	return packageTarGz, func() {
		os.RemoveAll(tmpDir)
	}, err
}

func createCodePackage(dest, codeTarGz, metadataPath string) error {
	tarGzArchive, err := os.Create(dest)
	if err != nil {
		return err
	}

	gz := gzip.NewWriter(tarGzArchive)
	defer gz.Close()

	t := tar.NewWriter(gz)
	defer t.Close()
	defer t.Flush()

	for _, fileToArchive := range []string{codeTarGz, metadataPath} {
		f, err := os.Open(fileToArchive)
		if err != nil {
			return fmt.Errorf("failed opening %s: %v", fileToArchive, err)
		}

		fi, err := f.Stat()
		if err != nil {
			return err
		}

		h, err := tar.FileInfoHeader(fi, fi.Name())
		if err != nil {
			return err
		}

		h.Name = fi.Name()

		if err := t.WriteHeader(h); err != nil {
			return err
		}

		if _, err := io.Copy(t, f); err != nil {
			return err
		}

	}
	return nil
}

func gzipDir(dir, dest string) error {
	tarGzArchive, err := os.Create(dest)
	if err != nil {
		return err
	}

	gz := gzip.NewWriter(tarGzArchive)
	defer gz.Close()

	t := tar.NewWriter(gz)
	defer t.Close()
	defer t.Flush()

	err = filepath.Walk(dir, func(path string, info fs.FileInfo, _ error) error {
		var f *os.File
		f, err = os.Open(path)
		if err != nil {
			return fmt.Errorf("failed opening %s: %v", path, err)
		}

		defer f.Close()

		h, err := tar.FileInfoHeader(info, info.Name())
		if err != nil {
			return err
		}

		parent := filepath.Dir(dir)
		h.Name = strings.Replace(path, parent, "", -1)
		err = t.WriteHeader(h)
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		_, err = io.Copy(t, f)
		if err != nil {
			return fmt.Errorf("failed reading data from %s: %v", f.Name(), err)
		}

		return nil
	})

	return err
}
