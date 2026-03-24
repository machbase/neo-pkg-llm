//go:build mage

package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const binaryName = "neo-pkg-llm"

// Build compiles the binary for the current platform.
func Build() error {
	fmt.Println("Building", binaryName, "...")
	cmd := exec.Command("go", "build", "-o", binaryName, ".")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}
	fmt.Println("Build complete:", binaryName)
	return nil
}

// Test runs all unit tests.
func Test() error {
	fmt.Println("Running tests...")
	cmd := exec.Command("go", "test", "-v", "./...")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tests failed: %w", err)
	}
	return nil
}

// Package cross-compiles and packages the binary for the given platform.
// Usage: go run mage.go package linux-amd64
// Supported platforms: linux-amd64, linux-arm64, darwin-amd64, darwin-arm64, windows-amd64
func Package(platform string) error {
	parts := strings.SplitN(platform, "-", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid platform %q — use format GOOS-GOARCH (e.g. linux-amd64)", platform)
	}
	goos, goarch := parts[0], parts[1]

	outBinary := binaryName
	if goos == "windows" {
		outBinary += ".exe"
	}

	fmt.Printf("Cross-compiling for %s/%s ...\n", goos, goarch)
	cmd := exec.Command("go", "build", "-o", outBinary, ".")
	cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	if goos == "windows" {
		return packageZip(outBinary, platform)
	}
	return packageTarGz(outBinary, platform)
}

func packageTarGz(binary, platform string) error {
	archiveName := fmt.Sprintf("%s-%s.tar.gz", binaryName, platform)
	fmt.Println("Creating", archiveName, "...")

	f, err := os.Create(archiveName)
	if err != nil {
		return err
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	if err := addFileToTar(tw, binary); err != nil {
		return err
	}
	if err := addDirToTar(tw, "configs"); err != nil {
		return err
	}

	os.Remove(binary)
	fmt.Println("Package ready:", archiveName)
	return nil
}

func packageZip(binary, platform string) error {
	archiveName := fmt.Sprintf("%s-%s.zip", binaryName, platform)
	fmt.Println("Creating", archiveName, "...")

	f, err := os.Create(archiveName)
	if err != nil {
		return err
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	if err := addFileToZip(zw, binary); err != nil {
		return err
	}
	if err := addDirToZip(zw, "configs"); err != nil {
		return err
	}

	os.Remove(binary)
	fmt.Println("Package ready:", archiveName)
	return nil
}

func addFileToTar(tw *tar.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}

	hdr := &tar.Header{
		Name: filepath.Base(path),
		Mode: int64(info.Mode()),
		Size: info.Size(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err = io.Copy(tw, f)
	return err
}

func addDirToTar(tw *tar.Writer, dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return tw.WriteHeader(&tar.Header{
				Typeflag: tar.TypeDir,
				Name:     path + "/",
				Mode:     0755,
			})
		}
		return addFileToTarWithPath(tw, path)
	})
}

func addDirToZip(zw *zip.Writer, dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			_, err := zw.Create(path + "/")
			return err
		}
		return addFileToZipWithPath(zw, path)
	})
}

func addFileToTarWithPath(tw *tar.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}

	hdr := &tar.Header{
		Name: path,
		Mode: int64(info.Mode()),
		Size: info.Size(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err = io.Copy(tw, f)
	return err
}

func addFileToZipWithPath(zw *zip.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w, err := zw.Create(path)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, f)
	return err
}

func addFileToZip(zw *zip.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w, err := zw.Create(filepath.Base(path))
	if err != nil {
		return err
	}
	_, err = io.Copy(w, f)
	return err
}
