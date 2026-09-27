package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"arctickit/internal/image"
	"arctickit/internal/iso"
	"arctickit/internal/logger"
)

type screen int

const (
	screenHome screen = iota
	screenSource
	screenImage
	screenIntegrate
	screenRemove
	screenCustomize
	screenBuild
	screenTools
)

type model struct {
	screen screen
	cursor int

	width  int
	height int

	isoFiles     []string
	selectedISO  string
	extractedDir string

	imageFile string
	imageType string

	indexes       []image.ImageIndex
	selectedIndex int

	working bool
	status  string
	errMsg  string
}

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("39"))

	subtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("39")).
			Bold(true).
			Padding(0, 1)

	normalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Padding(0, 1)

	sectionStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("39"))

	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))
)

var menuItems = []string{
	"Source",
	"Image",
	"Integrate",
	"Remove",
	"Customize",
	"Build",
	"Tools",
	"Exit",
}

type isoScanMsg struct {
	files []string
}

type extractMsg struct {
	result iso.Result
	err    error
}

type imageInfoMsg struct {
	indexes []image.ImageIndex
	err     error
}

func NewModel() tea.Model {
	return &model{}
}

func (m *model) Init() tea.Cmd {
	return scanISOs()
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case isoScanMsg:
		m.isoFiles = msg.files

		if len(m.isoFiles) == 0 {
			m.selectedISO = ""
			m.errMsg = "No ISO files found in the ISO folder."
		} else {
			m.errMsg = ""

			if m.selectedISO == "" {
				m.selectedISO = m.isoFiles[0]
			}

			m.status = fmt.Sprintf(
				"Found %d ISO file(s).",
				len(m.isoFiles),
			)
		}

	case extractMsg:
		m.working = false

		if msg.err != nil {
			m.errMsg = msg.err.Error()
			m.status = ""
			logger.Error(msg.err.Error())
			return m, nil
		}

		m.extractedDir = msg.result.Extracted
		m.imageFile = msg.result.ImageFile
		m.imageType = msg.result.ImageType

		m.status = fmt.Sprintf(
			"Found %s: %s",
			msg.result.ImageType,
			filepath.Base(msg.result.ImageFile),
		)

		m.errMsg = ""

		logger.Info(
			"Loading Windows image information",
		)

		m.working = true
		m.screen = screenImage
		m.cursor = 0

		return m, inspectImage(msg.result.ImageFile)

	case imageInfoMsg:
		m.working = false

		if msg.err != nil {
			m.errMsg = msg.err.Error()
			m.status = ""
			return m, nil
		}

		m.indexes = msg.indexes
		m.selectedIndex = 0
		m.cursor = 0

		m.status = fmt.Sprintf(
			"Found %d Windows edition(s).",
			len(m.indexes),
		)

		m.errMsg = ""

	case tea.KeyMsg:
		if m.working {
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}

			return m, nil
		}

		switch msg.String() {

		case "ctrl+c", "q":
			return m, tea.Quit

		case "esc":
			if m.screen != screenHome {
				m.screen = screenHome
				m.cursor = 0
				m.errMsg = ""
			}

		case "up", "k":
			m.moveCursor(-1)

		case "down", "j":
			m.moveCursor(1)

		case "enter":
			return m.handleEnter()

		case "r":
			if m.screen == screenSource {
				return m, scanISOs()
			}

		case "e":
			if m.screen == screenSource {
				return m.extractSelectedISO()
			}

		case "i":
			if m.screen == screenImage && m.imageFile != "" {
				m.working = true
				m.status = "Running DISM..."
				m.errMsg = ""

				return m, inspectImage(m.imageFile)
			}
		}
	}

	return m, nil
}

func (m *model) moveCursor(direction int) {
	var max int

	switch m.screen {

	case screenHome:
		max = len(menuItems) - 1

	case screenSource:
		max = len(m.isoFiles) - 1

	case screenImage:
		max = len(m.indexes) - 1

	default:
		max = 0
	}

	if max < 0 {
		m.cursor = 0
		return
	}

	m.cursor += direction

	if m.cursor < 0 {
		m.cursor = max
	}

	if m.cursor > max {
		m.cursor = 0
	}

	if m.screen == screenSource && len(m.isoFiles) > 0 {
		m.selectedISO = m.isoFiles[m.cursor]
	}

	if m.screen == screenImage && len(m.indexes) > 0 {
		m.selectedIndex = m.cursor
	}
}

func (m *model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.screen {

	case screenHome:

		switch m.cursor {

		case 0:
			m.screen = screenSource
			m.cursor = 0

		case 1:
			m.screen = screenImage
			m.cursor = 0

		case 2:
			m.screen = screenIntegrate
			m.cursor = 0

		case 3:
			m.screen = screenRemove
			m.cursor = 0

		case 4:
			m.screen = screenCustomize
			m.cursor = 0

		case 5:
			m.screen = screenBuild
			m.cursor = 0

		case 6:
			m.screen = screenTools
			m.cursor = 0

		case 7:
			return m, tea.Quit
		}

	case screenSource:
		if len(m.isoFiles) > 0 {
			return m.extractSelectedISO()
		}

	case screenImage:
		if len(m.indexes) > 0 {
			m.selectedIndex = m.cursor

			m.status = fmt.Sprintf(
				"Selected Index %d: %s",
				m.indexes[m.selectedIndex].Index,
				m.indexes[m.selectedIndex].Name,
			)

			logger.Info(m.status)
		}
	}

	return m, nil
}

func (m *model) extractSelectedISO() (tea.Model, tea.Cmd) {
	if m.selectedISO == "" {
		m.errMsg = "No ISO selected."
		return m, nil
	}

	root, err := executableRoot()

	if err != nil {
		m.errMsg = err.Error()
		return m, nil
	}

	extracted := filepath.Join(
		root,
		"work",
		"extracted",
	)

	m.working = true
	m.errMsg = ""
	m.status = "Mounting and extracting ISO..."

	logger.Info("Selected ISO: " + m.selectedISO)

	return m, extractISO(
		m.selectedISO,
		extracted,
	)
}

func (m *model) View() string {
	if m.width == 0 {
		return "Starting ArcticKit..."
	}

	switch m.screen {

	case screenSource:
		return m.sourceView()

	case screenImage:
		return m.imageView()

	case screenIntegrate:
		return m.moduleView(
			"Integrate",
			"Integrate drivers, updates, packages and features.",
		)

	case screenRemove:
		return m.moduleView(
			"Remove",
			"Remove Windows components and unwanted packages.",
		)

	case screenCustomize:
		return m.moduleView(
			"Customize",
			"Customize Windows settings and defaults.",
		)

	case screenBuild:
		return m.moduleView(
			"Build",
			"Commit changes and build the final Windows image.",
		)

	case screenTools:
		return m.moduleView(
			"Tools",
			"ArcticKit maintenance and diagnostic tools.",
		)

	default:
		return m.homeView()
	}
}

func (m *model) homeView() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("❄ ARCTICKIT"))
	b.WriteString("\n")
	b.WriteString(
		subtitleStyle.Render(
			"Windows Image Customization Toolkit",
		),
	)

	b.WriteString("\n\n")
	b.WriteString(sectionStyle.Render("MAIN MENU"))
	b.WriteString("\n\n")

	for i, item := range menuItems {
		if i == m.cursor {
			b.WriteString(selectedStyle.Render("❯ " + item))
		} else {
			b.WriteString(normalStyle.Render("  " + item))
		}

		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(sectionStyle.Render("WORKSPACE"))
	b.WriteString("\n")

	if m.selectedISO != "" {
		b.WriteString(
			normalStyle.Render(
				"ISO: " + filepath.Base(m.selectedISO),
			),
		)
	} else {
		b.WriteString(
			errorStyle.Render("ISO: Not selected"),
		)
	}

	b.WriteString("\n")

	if m.imageFile != "" {
		b.WriteString(
			normalStyle.Render(
				"Image: " + filepath.Base(m.imageFile),
			),
		)
	}

	b.WriteString("\n")

	if m.status != "" {
		b.WriteString(statusStyle.Render(m.status))
		b.WriteString("\n")
	}

	if m.errMsg != "" {
		b.WriteString(errorStyle.Render("⚠ " + m.errMsg))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(
		footerStyle.Render(
			"↑/↓ Navigate  •  Enter Select  •  Q Quit",
		),
	)

	return b.String()
}

func (m *model) sourceView() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("SOURCE"))
	b.WriteString("\n")

	b.WriteString(
		subtitleStyle.Render(
			"Select Windows installation media.",
		),
	)

	b.WriteString("\n\n")

	if len(m.isoFiles) == 0 {
		b.WriteString(
			errorStyle.Render(
				"⚠ No ISO files found.",
			),
		)

		b.WriteString("\n\n")

		b.WriteString(
			normalStyle.Render(
				"Put a Windows ISO here:",
			),
		)

		b.WriteString("\n")

		b.WriteString(
			normalStyle.Render(
				getISOFolder(),
			),
		)

		b.WriteString("\n\n")

		b.WriteString(
			footerStyle.Render(
				"R Rescan  •  Esc Back",
			),
		)

		return b.String()
	}

	b.WriteString(sectionStyle.Render("AVAILABLE ISOS"))
	b.WriteString("\n\n")

	for i, isoPath := range m.isoFiles {
		if i == m.cursor {
			b.WriteString(
				selectedStyle.Render(
					"❯ " + filepath.Base(isoPath),
				),
			)
		} else {
			b.WriteString(
				normalStyle.Render(
					"  " + filepath.Base(isoPath),
				),
			)
		}

		b.WriteString("\n")
	}

	b.WriteString("\n")

	b.WriteString(
		statusStyle.Render(
			"Selected: " + filepath.Base(m.selectedISO),
		),
	)

	b.WriteString("\n\n")

	b.WriteString(
		footerStyle.Render(
			"↑/↓ Select  •  Enter Extract & Inspect  •  E Extract  •  R Rescan  •  Esc Back",
		),
	)

	return b.String()
}

func (m *model) imageView() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("IMAGE"))
	b.WriteString("\n")

	b.WriteString(
		subtitleStyle.Render(
			"Windows image indexes detected by DISM.",
		),
	)

	b.WriteString("\n\n")

	if m.working {
		b.WriteString(
			statusStyle.Render(
				"⏳ " + m.status,
			),
		)

		b.WriteString("\n\n")

		b.WriteString(
			footerStyle.Render(
				"Please wait...",
			),
		)

		return b.String()
	}

	if m.imageFile == "" {
		b.WriteString(
			errorStyle.Render(
				"⚠ No Windows image has been loaded.",
			),
		)

		b.WriteString("\n\n")

		if m.selectedISO != "" {
			b.WriteString(
				normalStyle.Render(
					"Select the ISO from Source and press Enter.",
				),
			)
		} else {
			b.WriteString(
				normalStyle.Render(
					"Select an ISO from Source first.",
				),
			)
		}

		b.WriteString("\n\n")

		b.WriteString(
			footerStyle.Render(
				"Esc Back",
			),
		)

		return b.String()
	}

	b.WriteString(sectionStyle.Render("IMAGE"))
	b.WriteString("\n")

	b.WriteString(
		normalStyle.Render(
			filepath.Base(m.imageFile),
		),
	)

	b.WriteString("\n")

	b.WriteString(
		normalStyle.Render(
			"Type: " + m.imageType,
		),
	)

	b.WriteString("\n\n")

	b.WriteString(sectionStyle.Render("WINDOWS EDITIONS"))
	b.WriteString("\n\n")

	for i, img := range m.indexes {
		line := fmt.Sprintf(
			"[%d] %s",
			img.Index,
			img.Name,
		)

		if img.Description != "" &&
			img.Description != img.Name {
			line += " — " + img.Description
		}

		if i == m.cursor {
			b.WriteString(
				selectedStyle.Render("❯ " + line),
			)
		} else {
			b.WriteString(
				normalStyle.Render("  " + line),
			)
		}

		b.WriteString("\n")
	}

	b.WriteString("\n")

	if len(m.indexes) > 0 {
		selected := m.indexes[m.selectedIndex]

		b.WriteString(
			statusStyle.Render(
				fmt.Sprintf(
					"Selected: Index %d — %s",
					selected.Index,
					selected.Name,
				),
			),
		)

		b.WriteString("\n")
	}

	if m.errMsg != "" {
		b.WriteString(
			errorStyle.Render("⚠ " + m.errMsg),
		)

		b.WriteString("\n")
	}

	b.WriteString("\n")

	b.WriteString(
		footerStyle.Render(
			"↑/↓ Select  •  Enter Confirm  •  I Reinspect  •  Esc Back",
		),
	)

	return b.String()
}

func (m *model) moduleView(name, description string) string {
	var b strings.Builder

	b.WriteString(titleStyle.Render(name))
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render(description))
	b.WriteString("\n\n")

	if m.imageFile != "" {
		b.WriteString(sectionStyle.Render("CURRENT IMAGE"))
		b.WriteString("\n\n")

		b.WriteString(
			normalStyle.Render(
				"Image: " + filepath.Base(m.imageFile),
			),
		)

		b.WriteString("\n")

		if len(m.indexes) > 0 {
			img := m.indexes[m.selectedIndex]

			b.WriteString(
				normalStyle.Render(
					fmt.Sprintf(
						"Edition: Index %d — %s",
						img.Index,
						img.Name,
					),
				),
			)
		}
	} else {
		b.WriteString(
			errorStyle.Render(
				"⚠ No Windows image selected.",
			),
		)
	}

	b.WriteString("\n\n")

	b.WriteString(
		footerStyle.Render(
			"Esc Back  •  Q Quit",
		),
	)

	return b.String()
}

func scanISOs() tea.Cmd {
	return func() tea.Msg {
		root, err := executableRoot()

		if err != nil {
			return isoScanMsg{}
		}

		folder := filepath.Join(root, "ISO")

		var files []string

		for _, pattern := range []string{"*.iso", "*.ISO"} {
			matches, err := filepath.Glob(
				filepath.Join(folder, pattern),
			)

			if err == nil {
				files = append(files, matches...)
			}
		}

		logger.Info(
			fmt.Sprintf(
				"ISO scan complete: %d ISO(s) found",
				len(files),
			),
		)

		return isoScanMsg{
			files: files,
		}
	}
}

func extractISO(isoPath, destination string) tea.Cmd {
	return func() tea.Msg {
		if runtime.GOOS != "windows" {
			return extractMsg{
				err: fmt.Errorf(
					"ISO extraction currently requires Windows",
				),
			}
		}

		_ = os.RemoveAll(destination)

		result, err := iso.Extract(
			isoPath,
			destination,
		)

		return extractMsg{
			result: result,
			err:    err,
		}
	}
}

func inspectImage(path string) tea.Cmd {
	return func() tea.Msg {
		indexes, err := image.GetWimInfo(path)

		return imageInfoMsg{
			indexes: indexes,
			err:     err,
		}
	}
}

func executableRoot() (string, error) {
	exe, err := os.Executable()

	if err != nil {
		return "", fmt.Errorf(
			"unable to determine ArcticKit directory: %w",
			err,
		)
	}

	return filepath.Dir(exe), nil
}

func getISOFolder() string {
	root, err := executableRoot()

	if err != nil {
		return filepath.Join(".", "ISO")
	}

	return filepath.Join(root, "ISO")
}
