# ❄ ArcticKit

ArcticKit is a modular TUI-based Windows image customization toolkit.

The project is written in Go and uses Bubble Tea for its terminal user interface.

## Goals

ArcticKit is designed to provide an MSMG Toolkit-style workflow while using a modular architecture.

Planned functionality includes:

- Windows ISO detection
- ISO extraction
- install.wim detection
- install.esd detection
- WIM image enumeration
- Edition selection
- DISM integration
- Image mounting
- Image servicing
- Windows Update integration
- Driver integration
- Package integration
- Component removal
- Registry customization
- Service customization
- Default-user customization
- ISO rebuilding

## Requirements

- Windows
- Go 1.24 or newer
- Windows Terminal or another ANSI/Unicode-capable terminal

## Project Structure

```text
ArcticKit/
├── arctickit.exe
├── go.mod
├── go.sum
├── README.md
│
├── cmd/
│   └── arctickit/
│       └── main.go
│
├── internal/
│   ├── tui/
│   │   └── app.go
│   │
│   ├── image/
│   │   ├── image.go
│   │   └── dism.go
│   │
│   └── platform/
│       └── windows.go
│
├── ISO/
│   └── Windows.iso
│
├── work/
│   ├── extracted/
│   └── mount/
│
├── output/
│
├── logs/
│
└── config/
````

## Running

From the ArcticKit directory:

```powershell
.\arctickit.exe
```

Or run directly from source:

```powershell
go run ./cmd/arctickit
```

## ISO Files

Place Windows ISO files inside:

```text
ISO\
```

ArcticKit automatically scans the ISO directory when it starts.

## Current Features

The initial development version provides:

* Bubble Tea TUI
* Keyboard navigation
* ArcticKit menu
* ISO directory detection
* ISO file scanning
* Source selection placeholder
* Modular image backend structure
* Windows platform backend structure

## Keyboard Controls

```text
↑ / K       Move up
↓ / J       Move down
Enter       Select
Q           Quit
Ctrl+C      Quit
```

## Architecture

ArcticKit separates the TUI from the Windows image-management backend.

The TUI should not directly depend on DISM.

Instead, image operations will eventually be exposed through an image backend interface.

This will make it possible to add additional backends in the future.

## Planned Image Backend

Windows:

```text
ArcticKit
    │
    ├── TUI
    │
    └── Image Backend
            │
            └── DISM.exe
```

Future platforms could use different image-management tools without changing the TUI.

## Development

Run the automated checks and build with PowerShell:

```powershell
.\test.ps1
```

This runs `go test ./...`, `go vet ./...`, and builds
`arctickit.exe` in the project root. To also launch the TUI for a manual smoke test:

```powershell
.\test.ps1 -RunApp
```

Format the project:

```powershell
gofmt -w .\cmd .\internal
```

Build:

```powershell
go build -o arctickit.exe .\cmd\arctickit
```

Run:

```powershell
.\arctickit.exe
```

## License

License to be selected before the first public release.
