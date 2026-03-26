//go:build mage

package main

import (
	"archive/tar"
	"archive/zip"
	"bufio"
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
		Name:    filepath.Base(path),
		Mode:    int64(info.Mode()),
		Size:    info.Size(),
		ModTime: info.ModTime(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err = io.Copy(tw, f)
	return err
}


// Dp packages the binary for the current platform and deploys it via scp.
// Reads HOST, USER, PATH from .env file.
func Dp() error {
	env, err := loadEnvFile(".env")
	if err != nil {
		return fmt.Errorf("failed to load .env: %w", err)
	}
	host, ok1 := env["HOST"]
	user, ok2 := env["USER"]
	remotePath, ok3 := env["PATH"]
	if !ok1 || !ok2 || !ok3 {
		return fmt.Errorf(".env must contain HOST, USER, and PATH")
	}

	goos := os.Getenv("GOOS")
	if goos == "" {
		out, err := exec.Command("go", "env", "GOOS").Output()
		if err != nil {
			return fmt.Errorf("failed to get GOOS: %w", err)
		}
		goos = strings.TrimSpace(string(out))
	}
	goarch := os.Getenv("GOARCH")
	if goarch == "" {
		out, err := exec.Command("go", "env", "GOARCH").Output()
		if err != nil {
			return fmt.Errorf("failed to get GOARCH: %w", err)
		}
		goarch = strings.TrimSpace(string(out))
	}
	platform := goos + "-" + goarch

	if err := Package(platform); err != nil {
		return err
	}

	archive := fmt.Sprintf("%s-%s.tar.gz", binaryName, platform)
	dest := fmt.Sprintf("%s@%s:%s/%s", user, host, remotePath, archive)
	fmt.Printf("Deploying %s → %s\n", archive, dest)
	cmd := exec.Command("scp", archive, dest)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("scp failed: %w", err)
	}
	fmt.Println("Deploy complete.")
	return nil
}

func loadEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	env := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			env[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return env, scanner.Err()
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
