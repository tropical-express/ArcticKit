package image

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"arctickit/internal/logger"
)

type Progress struct {
	Stage   string
	Percent int
}

type MountedImage struct {
	MountPath string
	ImagePath string
	Index     int
	ReadWrite string
}

type RemovalOptions struct {
	DiagnosticPolicy  bool
	TelemetryServices bool
	FeedbackTasks     bool
	Xbox              bool
	Clipchamp         bool
	Teams             bool
	Solitaire         bool
	MixedReality      bool
	FeedbackHub       bool
}

func ApplyRemovals(imagePath string, index int, workDir string, options RemovalOptions) (string, error) {
	return ApplyRemovalsWithProgress(imagePath, index, workDir, options, nil)
}

func ApplyRemovalsWithProgress(imagePath string, index int, workDir string, options RemovalOptions, report func(Progress)) (string, error) {
	if runtime.GOOS != "windows" {
		return "", fmt.Errorf("offline Windows image servicing requires Windows")
	}
	if index < 1 {
		return "", fmt.Errorf("image index must be greater than zero")
	}
	if !options.anySelected() {
		return "", fmt.Errorf("no removal options selected")
	}
	logger.Info(fmt.Sprintf("Starting offline image servicing: image=%s index=%d", imagePath, index))
	if _, err := os.Stat(imagePath); err != nil {
		return "", fmt.Errorf("image not found: %w", err)
	}

	outputDir := filepath.Join(workDir, "modified", time.Now().Format("20060102-150405.000000000"))
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("creating modified image directory: %w", err)
	}
	logger.Info("Modified image output directory: " + outputDir)

	outputImage := filepath.Join(outputDir, "install.wim")
	switch strings.ToLower(filepath.Ext(imagePath)) {
	case ".wim":
		reportProgress(report, "Copying source WIM", 0)
		if err := copyFile(imagePath, outputImage); err != nil {
			return "", fmt.Errorf("copying source WIM: %w", err)
		}
		reportProgress(report, "Copying source WIM", 100)
	case ".esd":
		if err := runDISMProgress(report, "Exporting selected ESD edition",
			"/English",
			"/Export-Image",
			"/SourceImageFile:"+imagePath,
			"/SourceIndex:"+strconv.Itoa(index),
			"/DestinationImageFile:"+outputImage,
			"/Compress:max",
			"/CheckIntegrity",
		); err != nil {
			return "", fmt.Errorf("exporting selected ESD edition to WIM: %w", err)
		}
	default:
		return "", fmt.Errorf("unsupported image format: %s", filepath.Ext(imagePath))
	}

	mountDir := filepath.Join(outputDir, "mount")
	if err := os.MkdirAll(mountDir, 0755); err != nil {
		return "", fmt.Errorf("creating image mount directory: %w", err)
	}

	if err := runDISMProgress(report, "Mounting image",
		"/English",
		"/Mount-Image",
		"/ImageFile:"+outputImage,
		"/Index:"+strconv.Itoa(indexForMount(imagePath, index)),
		"/MountDir:"+mountDir,
	); err != nil {
		return "", fmt.Errorf("mounting WIM image: %w", err)
	}

	if err := applyMountedRemovals(mountDir, options, report); err != nil {
		discardErr := runDISMProgress(report, "Discarding incomplete changes", "/English", "/Unmount-Image", "/MountDir:"+mountDir, "/Discard")
		if discardErr != nil {
			return "", fmt.Errorf("%w; also failed to discard mounted image: %v", err, discardErr)
		}
		return "", err
	}

	if err := runDISMProgress(report, "Committing modified image", "/English", "/Unmount-Image", "/MountDir:"+mountDir, "/Commit"); err != nil {
		discardErr := runDISMProgress(report, "Discarding incomplete changes", "/English", "/Unmount-Image", "/MountDir:"+mountDir, "/Discard")
		if discardErr != nil {
			return "", fmt.Errorf("committing modified image: %w; discarding failed: %v", err, discardErr)
		}
		return "", fmt.Errorf("committing modified image: %w", err)
	}

	logger.Info("Offline image servicing completed: " + outputImage)
	return outputImage, nil
}

func GetMountedImages() ([]MountedImage, error) {
	if runtime.GOOS != "windows" {
		return nil, fmt.Errorf("mounted Windows image management requires Windows")
	}
	output, err := runDISMOutputProgress(nil, "Listing mounted images", "/English", "/Get-MountedWimInfo")
	if err != nil {
		return nil, fmt.Errorf("listing mounted images: %w", err)
	}
	return parseMountedImages(output)
}

func UnmountImageWithProgress(mountPath string, commit bool, report func(Progress)) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("mounted Windows image management requires Windows")
	}
	mountPath = filepath.Clean(strings.TrimSpace(mountPath))
	if mountPath == "" || mountPath == "." {
		return fmt.Errorf("mount path is empty")
	}
	action := "/Discard"
	stage := "Discarding changes from " + mountPath
	if commit {
		action = "/Commit"
		stage = "Committing changes to " + mountPath
	}
	logger.Info(fmt.Sprintf("Unmount requested: mount=%s commit=%t", mountPath, commit))
	if err := runDISMProgress(report, stage, "/English", "/Unmount-Image", "/MountDir:"+mountPath, action); err != nil {
		return fmt.Errorf("unmounting image at %s: %w", mountPath, err)
	}
	logger.Info(fmt.Sprintf("Unmount completed: mount=%s commit=%t", mountPath, commit))
	return nil
}

func parseMountedImages(output string) ([]MountedImage, error) {
	var images []MountedImage
	var current MountedImage
	hasData := false

	appendCurrent := func() {
		if current.MountPath != "" {
			images = append(images, current)
		}
		current = MountedImage{}
		hasData = false
	}

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if line == "" || strings.HasPrefix(line, "====") {
			if hasData && current.MountPath != "" && current.ImagePath != "" {
				appendCurrent()
			}
			continue
		}

		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch key {
		case "mount dir":
			if current.MountPath != "" {
				appendCurrent()
			}
			current.MountPath = value
			hasData = true
		case "image file":
			current.ImagePath = value
			hasData = true
		case "image index":
			index, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("invalid mounted image index %q: %w", value, err)
			}
			current.Index = index
			hasData = true
		case "mounted read/write":
			current.ReadWrite = value
			hasData = true
		}
	}
	if hasData && current.MountPath != "" && current.ImagePath != "" {
		appendCurrent()
	}
	return images, nil
}

func (o RemovalOptions) anySelected() bool {
	return o.DiagnosticPolicy ||
		o.TelemetryServices ||
		o.FeedbackTasks ||
		o.Xbox ||
		o.Clipchamp ||
		o.Teams ||
		o.Solitaire ||
		o.MixedReality ||
		o.FeedbackHub
}

func indexForMount(imagePath string, index int) int {
	if strings.EqualFold(filepath.Ext(imagePath), ".esd") {
		return 1
	}
	return index
}

func applyMountedRemovals(mountDir string, options RemovalOptions, report func(Progress)) error {
	if options.DiagnosticPolicy || options.TelemetryServices {
		reportProgress(report, "Applying telemetry policies", 0)
		if err := applyTelemetryRegistry(mountDir, options); err != nil {
			return err
		}
		reportProgress(report, "Applying telemetry policies", 100)
	}
	if options.FeedbackTasks {
		reportProgress(report, "Disabling feedback tasks", 0)
		if err := disableFeedbackTasks(mountDir); err != nil {
			return err
		}
		reportProgress(report, "Disabling feedback tasks", 100)
	}
	if options.Xbox || options.Clipchamp || options.Teams || options.Solitaire || options.MixedReality || options.FeedbackHub {
		if err := removeProvisionedApps(mountDir, options, report); err != nil {
			return err
		}
	}
	return nil
}

func removeProvisionedApps(mountDir string, options RemovalOptions, report func(Progress)) error {
	output, err := runDISMOutputProgress(report, "Scanning provisioned apps", "/English", "/Get-ProvisionedAppxPackages", "/Image:"+mountDir)
	if err != nil {
		return fmt.Errorf("querying provisioned apps: %w", err)
	}

	var packages []string
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok || !strings.EqualFold(strings.TrimSpace(key), "PackageName") {
			continue
		}
		packageName := strings.TrimSpace(value)
		if packageNameMatches(packageName, options) {
			packages = append(packages, packageName)
			logger.Info("Selected provisioned app for removal: " + packageName)
		}
	}

	for i, packageName := range packages {
		stage := fmt.Sprintf("Removing app %d/%d: %s", i+1, len(packages), packageName)
		reportProgress(report, stage, 0)
		appProgress := func(progress Progress) {
			progress.Stage = stage
			progress.Percent = (i*100 + progress.Percent) / len(packages)
			reportProgress(report, progress.Stage, progress.Percent)
		}
		if err := runDISMProgress(appProgress, stage,
			"/English",
			"/Remove-ProvisionedAppxPackage",
			"/PackageName:"+packageName,
			"/Image:"+mountDir,
		); err != nil {
			return fmt.Errorf("removing provisioned app %s: %w", packageName, err)
		}
	}
	logger.Info(fmt.Sprintf("Provisioned app removal complete: %d package(s) removed", len(packages)))
	if len(packages) == 0 {
		logger.Info("No matching provisioned apps were found")
		reportProgress(report, "No matching provisioned apps found", 100)
	}
	return nil
}

func packageNameMatches(packageName string, options RemovalOptions) bool {
	name := strings.ToLower(packageName)
	return options.Xbox && (strings.HasPrefix(name, "microsoft.xbox") || strings.HasPrefix(name, "microsoft.gamingapp")) ||
		options.Clipchamp && strings.HasPrefix(name, "clipchamp.clipchamp") ||
		options.Teams && (strings.HasPrefix(name, "microsoftteams") || strings.HasPrefix(name, "msteams")) ||
		options.Solitaire && strings.HasPrefix(name, "microsoft.microsoftsolitairecollection") ||
		options.MixedReality && strings.HasPrefix(name, "microsoft.mixedreality.portal") ||
		options.FeedbackHub && strings.HasPrefix(name, "microsoft.windowsfeedbackhub")
}

func applyTelemetryRegistry(mountDir string, options RemovalOptions) error {
	if options.DiagnosticPolicy {
		softwareHive := filepath.Join(mountDir, "Windows", "System32", "config", "SOFTWARE")
		if err := editOfflineHive(softwareHive, "Software", func(root string) error {
			key := root + `\Policies\Microsoft\Windows\DataCollection`
			return runReg("add", key, "/v", "AllowTelemetry", "/t", "REG_DWORD", "/d", "1", "/f")
		}); err != nil {
			return fmt.Errorf("setting minimum diagnostic data policy: %w", err)
		}
	}

	if options.TelemetryServices {
		systemHive := filepath.Join(mountDir, "Windows", "System32", "config", "SYSTEM")
		if err := editOfflineHive(systemHive, "System", func(root string) error {
			controlSet, err := currentControlSet(root)
			if err != nil {
				return err
			}
			serviceRoot := root + `\` + controlSet + `\Services`
			for _, service := range []string{"DiagTrack", "dmwappushservice"} {
				key := serviceRoot + `\` + service
				if err := runReg("query", key); err != nil {
					return fmt.Errorf("querying %s: %w", service, err)
				}
				if err := runReg("add", key, "/v", "Start", "/t", "REG_DWORD", "/d", "4", "/f"); err != nil {
					return fmt.Errorf("disabling %s in %s: %w", service, controlSet, err)
				}
			}
			return nil
		}); err != nil {
			return fmt.Errorf("disabling telemetry services: %w", err)
		}
	}
	return nil
}

func currentControlSet(root string) (string, error) {
	output, err := runRegOutput("query", root+`\Select`, "/v", "Current")
	if err != nil {
		return "", fmt.Errorf("reading active control set: %w", err)
	}
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 || !strings.EqualFold(fields[0], "Current") {
			continue
		}
		value, err := strconv.ParseUint(fields[len(fields)-1], 0, 32)
		if err != nil {
			return "", fmt.Errorf("parsing active control set %q: %w", fields[len(fields)-1], err)
		}
		return fmt.Sprintf("ControlSet%03d", value), nil
	}
	return "", fmt.Errorf("active control set was not present in offline SYSTEM hive")
}

var enabledTaskSetting = regexp.MustCompile(`(?i)<Enabled>\s*true\s*</Enabled>`)

func disableFeedbackTasks(mountDir string) error {
	tasks := []string{
		filepath.Join("Application Experience", "Microsoft Compatibility Appraiser"),
		filepath.Join("Application Experience", "ProgramDataUpdater"),
		filepath.Join("Application Experience", "StartupAppTask"),
		filepath.Join("Customer Experience Improvement Program", "Consolidator"),
		filepath.Join("Customer Experience Improvement Program", "KernelCeipTask"),
		filepath.Join("Customer Experience Improvement Program", "UsbCeip"),
		filepath.Join("Feedback", "Siuf", "DmClient"),
		filepath.Join("Feedback", "Siuf", "DmClientOnScenarioDownload"),
	}

	taskRoot := filepath.Join(mountDir, "Windows", "System32", "Tasks", "Microsoft", "Windows")
	for _, relativePath := range tasks {
		path := filepath.Join(taskRoot, relativePath)
		content, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("reading telemetry task %s: %w", relativePath, err)
		}
		text, encoding, err := decodeTask(content)
		if err != nil {
			return fmt.Errorf("decoding telemetry task %s: %w", relativePath, err)
		}
		if !enabledTaskSetting.MatchString(text) {
			continue
		}

		updatedText := enabledTaskSetting.ReplaceAllString(text, "<Enabled>false</Enabled>")
		updated, err := encodeTask(updatedText, encoding)
		if err != nil {
			return fmt.Errorf("encoding telemetry task %s: %w", relativePath, err)
		}
		if err := writeTaskFile(path, updated); err != nil {
			return fmt.Errorf("disabling telemetry task %s: %w", relativePath, err)
		}
		logger.Info("Disabled telemetry task: " + relativePath)
	}
	return nil
}

type taskEncoding int

const (
	taskUTF8 taskEncoding = iota
	taskUTF16LE
	taskUTF16BE
)

func decodeTask(content []byte) (string, taskEncoding, error) {
	if len(content) >= 2 && content[0] == 0xff && content[1] == 0xfe {
		if (len(content)-2)%2 != 0 {
			return "", 0, fmt.Errorf("invalid UTF-16LE task XML length")
		}
		units := make([]uint16, (len(content)-2)/2)
		for i := range units {
			units[i] = binary.LittleEndian.Uint16(content[2+i*2:])
		}
		return string(utf16.Decode(units)), taskUTF16LE, nil
	}
	if len(content) >= 2 && content[0] == 0xfe && content[1] == 0xff {
		if (len(content)-2)%2 != 0 {
			return "", 0, fmt.Errorf("invalid UTF-16BE task XML length")
		}
		units := make([]uint16, (len(content)-2)/2)
		for i := range units {
			units[i] = binary.BigEndian.Uint16(content[2+i*2:])
		}
		return string(utf16.Decode(units)), taskUTF16BE, nil
	}
	return string(content), taskUTF8, nil
}

func encodeTask(text string, encoding taskEncoding) ([]byte, error) {
	switch encoding {
	case taskUTF8:
		return []byte(text), nil
	case taskUTF16LE, taskUTF16BE:
		units := utf16.Encode([]rune(text))
		content := make([]byte, 2+len(units)*2)
		if encoding == taskUTF16LE {
			content[0], content[1] = 0xff, 0xfe
			for i, unit := range units {
				binary.LittleEndian.PutUint16(content[2+i*2:], unit)
			}
		} else {
			content[0], content[1] = 0xfe, 0xff
			for i, unit := range units {
				binary.BigEndian.PutUint16(content[2+i*2:], unit)
			}
		}
		return content, nil
	default:
		return nil, fmt.Errorf("unsupported task XML encoding")
	}
}

func editOfflineHive(hivePath, name string, edit func(string) error) (err error) {
	if _, err := os.Stat(hivePath); err != nil {
		return fmt.Errorf("offline registry hive not found: %w", err)
	}

	root := `HKLM\ArcticKit` + name + strconv.Itoa(os.Getpid())
	if err := runReg("load", root, hivePath); err != nil {
		return err
	}
	defer func() {
		if unloadErr := runReg("unload", root); unloadErr != nil {
			if err == nil {
				err = fmt.Errorf("unloading offline registry hive: %w", unloadErr)
			} else {
				err = fmt.Errorf("%w; unloading offline registry hive failed: %v", err, unloadErr)
			}
		}
	}()

	return edit(root)
}

func runReg(args ...string) error {
	_, err := runRegOutput(args...)
	return err
}

func runRegOutput(args ...string) (string, error) {
	cmd := exec.Command("reg.exe", args...)
	logger.Command("reg.exe", args...)
	started := time.Now()
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Error(fmt.Sprintf("Registry command failed after %s: %v", time.Since(started).Round(time.Millisecond), err))
		if len(output) > 0 {
			logger.Error("Registry command output: " + strings.TrimSpace(string(output)))
		}
		return string(output), fmt.Errorf("reg.exe %s failed: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	logger.Info(fmt.Sprintf("Registry command completed in %s", time.Since(started).Round(time.Millisecond)))
	return string(output), nil
}

func reportProgress(report func(Progress), stage string, percent int) {
	if report != nil {
		report(Progress{Stage: stage, Percent: percent})
	}
}

func runDISMProgress(report func(Progress), stage string, args ...string) error {
	_, err := runDISMOutputProgress(report, stage, args...)
	return err
}

func runDISMOutputProgress(report func(Progress), stage string, args ...string) (string, error) {
	cmd := exec.Command("dism.exe", args...)
	logger.Command("dism.exe", args...)
	logger.Info("DISM started: " + stage)
	started := time.Now()
	reportProgress(report, stage, 0)

	var output bytes.Buffer
	progressOutput := &dismProgressWriter{
		output: &output,
		report: report,
		stage:  stage,
	}
	cmd.Stdout = progressOutput
	cmd.Stderr = progressOutput
	err := cmd.Run()
	duration := time.Since(started).Round(time.Millisecond)
	if err != nil {
		logger.Error(fmt.Sprintf("DISM failed: stage=%s duration=%s error=%v", stage, duration, err))
		if output.Len() > 0 {
			logger.Error("DISM output: " + truncateLog(strings.TrimSpace(output.String()), 16*1024))
		}
		return output.String(), fmt.Errorf("dism.exe %s failed: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(output.String()))
	}
	reportProgress(report, stage, 100)
	logger.Info(fmt.Sprintf("DISM completed: stage=%s duration=%s", stage, duration))
	return output.String(), nil
}

type dismProgressWriter struct {
	mu       sync.Mutex
	output   *bytes.Buffer
	report   func(Progress)
	stage    string
	lastSeen int
	pending  string
}

var dismPercentPattern = regexp.MustCompile(`(\d{1,3}(?:\.\d+)?)%`)

func (w *dismProgressWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	_, _ = w.output.Write(data)
	w.pending += string(data)
	matches := dismPercentPattern.FindAllStringSubmatch(w.pending, -1)
	for _, match := range matches {
		value, err := strconv.ParseFloat(match[1], 64)
		if err != nil {
			continue
		}
		percent := int(math.Round(value))
		if percent > 100 || percent <= w.lastSeen {
			continue
		}
		w.lastSeen = percent
		reportProgress(w.report, w.stage, percent)
		if percent == 0 || percent == 100 || percent%10 == 0 {
			logger.Info(fmt.Sprintf("DISM progress: stage=%s percent=%d", w.stage, percent))
		}
	}
	if len(w.pending) > 64 {
		w.pending = w.pending[len(w.pending)-64:]
	}
	return len(data), nil
}

func truncateLog(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max] + "\n[output truncated]"
}

func copyFile(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()

	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func writeTaskFile(path string, content []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path, content, info.Mode().Perm())
}
