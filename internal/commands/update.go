package commands

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const releaseAPI = "https://api.github.com/repos/yun-lun-li/devcontainer-helper/releases/latest"

// version is set for release builds. Source builds report "dev".
var version = "dev"

type githubRelease struct {
	TagName string `json:"tag_name"`
}

func update() error {
	fmt.Printf("Installed Version: %s\n", strings.TrimPrefix(version, "v"))
	release, err := latestRelease(2 * time.Minute)
	if err != nil {
		return err
	}
	if normalizeVersion(version) == normalizeVersion(release.TagName) {
		fmt.Printf("dch %s is already up to date\n", normalizeVersion(version))
		return nil
	}

	if err := withSpinner("Installing update", func() error {
		goos, goarch := runtime.GOOS, runtime.GOARCH
		if (goos != "linux" && goos != "darwin") || (goarch != "amd64" && goarch != "arm64") {
			return fmt.Errorf("updates are not available for %s-%s", goos, goarch)
		}
		archiveName := fmt.Sprintf("dch-%s-%s-%s.tar.gz", release.TagName, goos, goarch)
		archiveURL := "https://github.com/yun-lun-li/devcontainer-helper/releases/download/" + release.TagName + "/" + archiveName
		request, err := http.NewRequest(http.MethodGet, archiveURL, nil)
		if err != nil {
			return err
		}
		client := &http.Client{Timeout: 2 * time.Minute}
		response, err := client.Do(request)
		if err != nil {
			return fmt.Errorf("could not download dch %s: %w", release.TagName, err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return fmt.Errorf("could not download dch %s: GitHub returned %s", release.TagName, response.Status)
		}

		archive, err := gzip.NewReader(response.Body)
		if err != nil {
			return fmt.Errorf("could not open release archive: %w", err)
		}
		defer archive.Close()
		tr := tar.NewReader(archive)
		var binary []byte
		expectedPath := fmt.Sprintf("dch-%s-%s-%s/dch", release.TagName, goos, goarch)
		for {
			header, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return fmt.Errorf("could not read release archive: %w", err)
			}
			if filepath.Clean(header.Name) != expectedPath || header.Typeflag != tar.TypeReg {
				continue
			}
			if header.Size <= 0 || header.Size > 128<<20 {
				return fmt.Errorf("release binary has an invalid size")
			}
			binary, err = io.ReadAll(io.LimitReader(tr, 128<<20))
			if err != nil {
				return fmt.Errorf("could not extract dch binary: %w", err)
			}
			break
		}
		if len(binary) == 0 {
			return fmt.Errorf("release archive does not contain %s", expectedPath)
		}

		executable, err := os.Executable()
		if err != nil {
			return fmt.Errorf("could not locate the current dch executable: %w", err)
		}
		if resolved, err := filepath.EvalSymlinks(executable); err == nil {
			executable = resolved
		}
		info, err := os.Stat(executable)
		if err != nil {
			return fmt.Errorf("could not inspect %s: %w", executable, err)
		}
		tmp, err := os.CreateTemp(filepath.Dir(executable), ".dch-update-*")
		if err != nil {
			return fmt.Errorf("could not create update file beside %s: %w", executable, err)
		}
		tmpPath := tmp.Name()
		defer os.Remove(tmpPath)
		if _, err := tmp.Write(binary); err != nil {
			tmp.Close()
			return fmt.Errorf("could not write updated binary: %w", err)
		}
		if err := tmp.Chmod(info.Mode().Perm()); err != nil {
			tmp.Close()
			return fmt.Errorf("could not set updated binary permissions: %w", err)
		}
		if err := tmp.Close(); err != nil {
			return fmt.Errorf("could not finish updated binary: %w", err)
		}
		if err := os.Rename(tmpPath, executable); err != nil {
			return fmt.Errorf("could not replace %s: %w", executable, err)
		}
		return nil
	}); err != nil {
		return err
	}
	fmt.Printf("Updated dch %s → %s\n", strings.TrimPrefix(version, "v"), strings.TrimPrefix(release.TagName, "v"))
	return nil
}

func latestRelease(timeout time.Duration) (githubRelease, error) {
	var release githubRelease
	if err := withSpinner("Checking for the latest dch release", func() error {
		client := &http.Client{Timeout: timeout}
		request, err := http.NewRequest(http.MethodGet, releaseAPI, nil)
		if err != nil {
			return err
		}
		request.Header.Set("Accept", "application/vnd.github+json")
		response, err := client.Do(request)
		if err != nil {
			return fmt.Errorf("could not check the latest release: %w", err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return fmt.Errorf("could not check the latest release: GitHub returned %s", response.Status)
		}
		if err := json.NewDecoder(response.Body).Decode(&release); err != nil {
			return fmt.Errorf("could not read latest release information: %w", err)
		}
		if release.TagName == "" {
			return fmt.Errorf("latest release did not include a version tag")
		}
		return nil
	}); err != nil {
		return githubRelease{}, err
	}
	return release, nil
}

func checkForStartupUpdate() {
	if version == "dev" {
		return
	}
	info, err := os.Stdin.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return
	}

	release, err := latestRelease(5 * time.Second)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not check for dch updates: %v\n", err)
		return
	}
	if normalizeVersion(version) == normalizeVersion(release.TagName) {
		fmt.Printf("dch %s is up to date\n", normalizeVersion(version))
		return
	}

	fmt.Printf("A new dch version is available: %s → %s. Update now? [y/N] ", normalizeVersion(version), normalizeVersion(release.TagName))
	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && len(answer) == 0 {
		return
	}
	if strings.EqualFold(strings.TrimSpace(answer), "y") || strings.EqualFold(strings.TrimSpace(answer), "yes") {
		if err := update(); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
		}
	}
}

func normalizeVersion(value string) string {
	return strings.TrimPrefix(value, "v")
}

func withSpinner(label string, action func() error) error {
	info, err := os.Stdout.Stat()
	interactive := err == nil && info.Mode()&os.ModeCharDevice != 0 && os.Getenv("TERM") != "dumb"
	if !interactive {
		fmt.Printf("%s...\n", label)
		return action()
	}

	frames := []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}
	done := make(chan struct{})
	var once sync.Once
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for i := 0; ; i++ {
			select {
			case <-done:
				return
			case <-ticker.C:
				fmt.Printf("\r%s %c", label, frames[i%len(frames)])
			}
		}
	}()
	result := action()
	once.Do(func() { close(done) })
	wg.Wait()
	if result != nil {
		fmt.Printf("\r\033[2K%s failed\n", label)
		return result
	}
	fmt.Printf("\r\033[2K%s ✓\n", label)
	return nil
}
