// Command winres writes app/rsrc_windows_amd64.syso: the app icon, a manifest (per-monitor DPI,
// current common controls) and the version info that Windows shows in the file's Properties. The Go
// linker picks the .syso file up by its name. Run before building the app; the file is not committed.
//
//	go run ./winres <repo root> <version>
package main

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/tc-hib/winres"
	"github.com/tc-hib/winres/version"
)

func main() {
	if len(os.Args) != 3 {
		log.Fatal("usage: winres <repo root> <version>")
	}
	root, ver := os.Args[1], os.Args[2]
	data, err := os.ReadFile(filepath.Join(root, "build", "windows", "icon.ico"))
	if err != nil {
		log.Fatal(err)
	}
	icon, err := winres.LoadICO(bytes.NewReader(data))
	if err != nil {
		log.Fatal(err)
	}
	rs := &winres.ResourceSet{}
	if err := rs.SetIcon(winres.Name("APP"), icon); err != nil {
		log.Fatal(err)
	}
	rs.SetManifest(winres.AppManifest{
		Identity:            winres.AssemblyIdentity{Name: "Xegesis.App", Version: [4]uint16{1, 0, 0, 0}},
		Description:         "Xegesis Bible study",
		Compatibility:       winres.Win7AndAbove,
		ExecutionLevel:      winres.AsInvoker,
		DPIAwareness:        winres.DPIPerMonitorV2,
		UIAccess:            false,
		UseCommonControlsV6: true,
	})
	vi := version.Info{}
	nums := regexp.MustCompile(`\d+`).FindAllString(ver, 3)
	fv := ""
	for i := range 3 {
		n := "0"
		if i < len(nums) {
			n = nums[i]
		}
		if _, err := strconv.Atoi(n); err != nil {
			n = "0"
		}
		fv += n + "."
	}
	fv += "0"
	vi.SetFileVersion(fv)
	vi.SetProductVersion(fv)
	const lang = 0x409
	vi.Set(lang, version.ProductName, "Xegesis")
	vi.Set(lang, version.FileDescription, "Xegesis Bible study")
	vi.Set(lang, version.LegalCopyright, "MIT OR Apache-2.0")
	vi.Set(lang, version.OriginalFilename, "Xegesis.exe")
	vi.Set(lang, version.ProductVersion, ver)
	rs.SetVersionInfo(vi)
	out, err := os.Create(filepath.Join(root, "app", "rsrc_windows_amd64.syso"))
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()
	if err := rs.WriteObject(out, winres.ArchAMD64); err != nil {
		log.Fatal(err)
	}
}
