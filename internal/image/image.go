package image

// Image represents a Windows installation image.
//
// This will eventually contain information such as:
//
//   - WIM/ESD path
//   - Image index
//   - Edition name
//   - Architecture
//   - Version
//   - Build number
//   - Mount path
type ImageIndex struct {
	Index       int
	Name        string
	Description string
}

type Image struct {
	Path         string
	Index        int
	Name         string
	Description  string
	Architecture string
	Version      string
	Build        string
	MountPath    string
}

func GetWimInfo(path string) ([]ImageIndex, error) {
	return nil, nil
}
