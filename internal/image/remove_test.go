package image

import (
	"bytes"
	"testing"
)

func TestPackageNameMatches(t *testing.T) {
	tests := []struct {
		name    string
		options RemovalOptions
		want    bool
	}{
		{"Microsoft.XboxApp_1.0.0.0_neutral__8wekyb3d8bbwe", RemovalOptions{Xbox: true}, true},
		{"Microsoft.GamingApp_1.0.0.0_neutral__8wekyb3d8bbwe", RemovalOptions{Xbox: true}, true},
		{"Microsoft.GamingServices_1.0.0.0_neutral__8wekyb3d8bbwe", RemovalOptions{Xbox: true}, false},
		{"Clipchamp.Clipchamp_1.0.0.0_neutral__8wekyb3d8bbwe", RemovalOptions{Clipchamp: true}, true},
		{"MSTeams_1.0.0.0_neutral__8wekyb3d8bbwe", RemovalOptions{Teams: true}, true},
		{"Microsoft.MicrosoftSolitaireCollection_1.0.0.0_neutral__8wekyb3d8bbwe", RemovalOptions{Solitaire: true}, true},
		{"Microsoft.MixedReality.Portal_1.0.0.0_neutral__8wekyb3d8bbwe", RemovalOptions{MixedReality: true}, true},
		{"Microsoft.WindowsFeedbackHub_1.0.0.0_neutral__8wekyb3d8bbwe", RemovalOptions{FeedbackHub: true}, true},
		{"Microsoft.WindowsNotepad_1.0.0.0_neutral__8wekyb3d8bbwe", RemovalOptions{FeedbackHub: true}, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := packageNameMatches(test.name, test.options); got != test.want {
				t.Errorf("packageNameMatches(%q) = %t, want %t", test.name, got, test.want)
			}
		})
	}
}

func TestRemovalOptionsAnySelected(t *testing.T) {
	if (RemovalOptions{}).anySelected() {
		t.Fatal("empty removal options should not select any operation")
	}
	if !(RemovalOptions{DiagnosticPolicy: true}).anySelected() {
		t.Fatal("selected removal option should be detected")
	}
}

func TestDecodeEncodeUTF16TaskXML(t *testing.T) {
	original, err := encodeTask("<?xml version=\"1.0\"?><Enabled>true</Enabled>", taskUTF16LE)
	if err != nil {
		t.Fatal(err)
	}

	text, encoding, err := decodeTask(original)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := encodeTask(enabledTaskSetting.ReplaceAllString(text, "<Enabled>false</Enabled>"), encoding)
	if err != nil {
		t.Fatal(err)
	}
	decoded, _, err := decodeTask(updated)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(updated, []byte("<Enabled>false</Enabled>")) {
		t.Fatal("UTF-16 XML should remain encoded as UTF-16")
	}
	if want := "<Enabled>false</Enabled>"; !bytes.Contains([]byte(decoded), []byte(want)) {
		t.Fatalf("updated XML = %q, want it to contain %q", decoded, want)
	}
	if !bytes.HasPrefix(updated, []byte{0xff, 0xfe}) {
		t.Fatal("UTF-16LE BOM was not preserved")
	}
}

func TestDISMProgressWriterParsesProgressAcrossChunks(t *testing.T) {
	var output bytes.Buffer
	var progress []Progress
	writer := &dismProgressWriter{
		output: &output,
		stage:  "Mounting image",
		report: func(value Progress) {
			progress = append(progress, value)
		},
	}

	chunks := []string{
		"[==== 12.",
		"5% ====]\r[======= 75% =======]\r[========== 100.0% =========]",
	}
	for _, chunk := range chunks {
		if _, err := writer.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}

	if len(progress) != 3 {
		t.Fatalf("got %d progress updates, want 3: %#v", len(progress), progress)
	}
	want := []int{13, 75, 100}
	for i, update := range progress {
		if update.Stage != "Mounting image" || update.Percent != want[i] {
			t.Errorf("progress[%d] = %#v, want stage Mounting image at %d%%", i, update, want[i])
		}
	}
	if output.Len() == 0 {
		t.Fatal("DISM output should be retained")
	}
}

func TestParseMountedImages(t *testing.T) {
	output := `
Deployment Image Servicing and Management tool

Mounted images:

Mount Dir : C:\work\mount-one
Image File : D:\sources\install.wim
Image Index : 3
Mounted Read/Write : Yes

Mount Dir : C:\work\mount-two
Image File : D:\sources\other.wim
Image Index : 1
Mounted Read/Write : No
`

	images, err := parseMountedImages(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 2 {
		t.Fatalf("got %d mounted images, want 2: %#v", len(images), images)
	}
	if images[0] != (MountedImage{
		MountPath: "C:\\work\\mount-one",
		ImagePath: "D:\\sources\\install.wim",
		Index:     3,
		ReadWrite: "Yes",
	}) {
		t.Errorf("first mounted image = %#v", images[0])
	}
	if images[1].MountPath != "C:\\work\\mount-two" || images[1].Index != 1 {
		t.Errorf("second mounted image = %#v", images[1])
	}
}

func TestParseMountedImagesRejectsInvalidIndex(t *testing.T) {
	_, err := parseMountedImages("Mount Dir : C:\\mount\nImage File : C:\\install.wim\nImage Index : invalid\n")
	if err == nil {
		t.Fatal("expected invalid image index to fail")
	}
}
