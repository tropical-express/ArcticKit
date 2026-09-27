package iso

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"arctickit/internal/logger"
)

type Result struct {
	ISOPath   string
	Extracted string
	ImageFile string
	ImageType string
}

func Extract(isoPath, destination string) (Result, error) {
	logger.Info("Starting ISO extraction")
	logger.Info("ISO: " + isoPath)
	logger.Info("Destination: " + destination)

	if _, err := os.Stat(isoPath); err != nil {
		return Result{}, fmt.Errorf("ISO not found: %w", err)
	}

	if err := os.MkdirAll(destination, 0755); err != nil {
		return Result{}, fmt.Errorf("creating extraction directory: %w", err)
	}

	ps := `
$ErrorActionPreference = "Stop"

$iso = Mount-DiskImage -ImagePath $args[0] -PassThru

try {
    $volume = $iso | Get-Volume

    if (-not $volume) {
        throw "Unable to determine mounted ISO volume."
    }

    $drive = $volume.DriveLetter + ":\"

    Write-Output $drive

    robocopy $drive $args[1] /E /COPY:DAT /DCOPY:DAT /R:2 /W:2 /NFL /NDL /NJH /NJS

    if ($LASTEXITCODE -gt 7) {
        throw "robocopy failed with exit code $LASTEXITCODE"
    }
}
finally {
    Dismount-DiskImage -ImagePath $args[0]
}
`

	logger.Info("Mounting ISO")

	cmd := exec.Command(
		"powershell.exe",
		"-NoProfile",
		"-NonInteractive",
		"-Command",
		ps,
		isoPath,
		destination,
	)

	output, err := cmd.CombinedOutput()

	if len(output) > 0 {
		logger.Info(strings.TrimSpace(string(output)))
	}

	if err != nil {
		logger.Error("ISO extraction failed: " + err.Error())

		return Result{}, fmt.Errorf(
			"ISO extraction failed: %w\n%s",
			err,
			strings.TrimSpace(string(output)),
		)
	}

	logger.Info("ISO extraction completed")

	imageFile, imageType, err := FindWindowsImage(destination)

	if err != nil {
		return Result{}, err
	}

	logger.Info("Windows image detected: " + imageFile)
	logger.Info("Image type: " + imageType)

	return Result{
		ISOPath:   isoPath,
		Extracted: destination,
		ImageFile: imageFile,
		ImageType: imageType,
	}, nil
}

func FindWindowsImage(root string) (string, string, error) {
	wim := filepath.Join(
		root,
		"sources",
		"install.wim",
	)

	esd := filepath.Join(
		root,
		"sources",
		"install.esd",
	)

	if _, err := os.Stat(wim); err == nil {
		return wim, "WIM", nil
	}

	if _, err := os.Stat(esd); err == nil {
		return esd, "ESD", nil
	}

	return "", "", fmt.Errorf(
		"could not find sources\\install.wim or sources\\install.esd",
	)
}
