package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"debug/pe"
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Oranjekop/TextViewer/internal/releaseinfo"
)

//go:embed windows-installer.nsi
var installerTemplate string

var bundleFiles = []string{
	"TextViewer.exe", "TextViewerShell.dll", "TextViewer.ContextMenu.msix",
	"TextViewer.ContextMenu.cer", "ContextMenu.ps1",
}

type configuration struct {
	Name, Identifier, Version, Icon string
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: release prepare|package -tag vMAJOR.MINOR.PATCH")
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	tag := flags.String("tag", "", "stable release tag")
	build := flags.String("build", "dist/native", "MyGo build output")
	output := flags.String("output", "dist/release", "release assets directory")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	version, err := releaseinfo.TagVersion(*tag)
	if err != nil {
		return err
	}
	switch args[0] {
	case "prepare":
		return prepareVersion("mygo.json", version)
	case "package":
		config, err := readConfiguration()
		if err != nil {
			return err
		}
		if version != config.Version {
			return fmt.Errorf("tag %s does not match mygo.json %s; run prepare first", *tag, config.Version)
		}
		return packageRelease(config, *build, *output)
	default:
		return fmt.Errorf("unknown release command %q", args[0])
	}
}

func prepareVersion(path, version string) error {
	if _, err := releaseinfo.ParseVersion(version); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var config map[string]json.RawMessage
	if err = json.Unmarshal(data, &config); err != nil {
		return err
	}
	config["version"], _ = json.Marshal(version)
	data, err = json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}

func readConfiguration() (configuration, error) {
	var config configuration
	data, err := os.ReadFile("mygo.json")
	if err != nil {
		return config, err
	}
	if err = json.Unmarshal(data, &config); err != nil {
		return config, err
	}
	_, err = releaseinfo.ParseVersion(config.Version)
	return config, err
}

func packageRelease(config configuration, build, output string) error {
	if err := os.MkdirAll(output, 0755); err != nil {
		return err
	}
	var names []string
	for _, arch := range []string{"amd64", "arm64"} {
		dir := filepath.Join(build, "windows-"+arch)
		if err := validatePayload(dir, config.Version, arch); err != nil {
			return err
		}
		name := releaseinfo.ArchiveName(config.Version, arch)
		if err := writeArchive(dir, filepath.Join(output, name)); err != nil {
			return err
		}
		names = append(names, name)
	}
	name := releaseinfo.InstallerName(config.Version)
	if err := buildInstaller(config, build, filepath.Join(output, name), false); err != nil {
		return err
	}
	names = append(names, name)
	slices.Sort(names)
	var checksums strings.Builder
	for _, name := range names {
		f, err := os.Open(filepath.Join(output, name))
		if err != nil {
			return err
		}
		hash := sha256.New()
		_, err = io.Copy(hash, f)
		f.Close()
		if err != nil {
			return err
		}
		fmt.Fprintf(&checksums, "%s  %s\n", hex.EncodeToString(hash.Sum(nil)), name)
	}
	return os.WriteFile(filepath.Join(output, "SHA256SUMS.txt"), []byte(checksums.String()), 0644)
}

func validatePayload(dir, version, arch string) error {
	machine, manifestArch := uint16(pe.IMAGE_FILE_MACHINE_AMD64), "x64"
	if arch == "arm64" {
		machine, manifestArch = pe.IMAGE_FILE_MACHINE_ARM64, "arm64"
	} else if arch != "amd64" {
		return fmt.Errorf("unsupported Windows architecture %q", arch)
	}
	for _, name := range []string{"TextViewer.exe", "TextViewerShell.dll"} {
		binary, err := pe.Open(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("validate %s %s: %w", arch, name, err)
		}
		actual := binary.Machine
		binary.Close()
		if actual != machine {
			return fmt.Errorf("%s %s has PE machine %x, expected %x", arch, name, actual, machine)
		}
	}
	pkg, err := zip.OpenReader(filepath.Join(dir, "TextViewer.ContextMenu.msix"))
	if err != nil {
		return err
	}
	defer pkg.Close()
	signed, manifestFound := false, false
	for _, entry := range pkg.File {
		if entry.Name == "AppxSignature.p7x" && entry.UncompressedSize64 > 0 {
			signed = true
		}
		if entry.Name != "AppxManifest.xml" {
			continue
		}
		file, err := entry.Open()
		if err != nil {
			return err
		}
		data, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
		file.Close()
		if err != nil || len(data) > 1<<20 {
			return fmt.Errorf("invalid %s identity manifest", arch)
		}
		if err := validateIdentityManifest(data, version, manifestArch); err != nil {
			return err
		}
		manifestFound = true
	}
	if !manifestFound || !signed {
		return fmt.Errorf("%s identity package is missing its manifest or signature", arch)
	}
	return nil
}

func validateIdentityManifest(data []byte, version, arch string) error {
	var manifest struct {
		Identity struct {
			Name         string `xml:"Name,attr"`
			Version      string `xml:"Version,attr"`
			Architecture string `xml:"ProcessorArchitecture,attr"`
		} `xml:"Identity"`
	}
	if err := xml.Unmarshal(data, &manifest); err != nil {
		return err
	}
	identity := manifest.Identity
	if identity.Name != "Oranjekop.TextViewer.ContextMenu" || identity.Version != version+".0" || identity.Architecture != arch {
		return fmt.Errorf("MSIX identity does not match TextViewer %s %s", version, arch)
	}
	return nil
}

func writeArchive(dir, target string) (err error) {
	f, err := os.Create(target)
	if err != nil {
		return err
	}
	defer func() {
		f.Close()
		if err != nil {
			os.Remove(target)
		}
	}()
	writer := zip.NewWriter(f)
	for _, name := range bundleFiles {
		source, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			writer.Close()
			return err
		}
		info, err := source.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			source.Close()
			writer.Close()
			return fmt.Errorf("invalid bundle file %s", name)
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			source.Close()
			writer.Close()
			return err
		}
		header.Name, header.Method = name, zip.Deflate
		entry, err := writer.CreateHeader(header)
		if err == nil {
			_, err = io.Copy(entry, source)
		}
		source.Close()
		if err != nil {
			writer.Close()
			return err
		}
	}
	if err = writer.Close(); err != nil {
		return err
	}
	return f.Close()
}

func buildInstaller(config configuration, build, target string, isolated bool) error {
	tool, err := findNSIS()
	if err != nil {
		return err
	}
	work, err := os.MkdirTemp("", "textviewer-installer-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	png, err := os.ReadFile(config.Icon)
	if err != nil {
		return err
	}
	icon := filepath.Join(work, "icon.ico")
	if err = os.WriteFile(icon, pngIcon(png), 0600); err != nil {
		return err
	}
	script, err := installerScript(config, build, target, icon, isolated)
	if err != nil {
		return err
	}
	path := filepath.Join(work, "installer.nsi")
	if err = os.WriteFile(path, append([]byte{0xef, 0xbb, 0xbf}, []byte(script)...), 0600); err != nil {
		return err
	}
	command := exec.Command(tool, "-V2", path)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("build installer: %w\n%s", err, output)
	}
	fmt.Println("Built installer:", target)
	return nil
}

func installerScript(config configuration, build, target, icon string, isolated bool) (string, error) {
	var payload [2]string
	for index, arch := range []string{"amd64", "arm64"} {
		var files strings.Builder
		for _, name := range bundleFiles {
			path, err := filepath.Abs(filepath.Join(build, "windows-"+arch, name))
			if err != nil {
				return "", err
			}
			if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
				return "", fmt.Errorf("missing %s %s", arch, name)
			}
			fmt.Fprintf(&files, "    File %s\n", nsisQuote(path))
		}
		payload[index] = files.String()
	}
	target, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	hooks, err := filepath.Abs("shell/installer.nsh")
	if err != nil {
		return "", err
	}
	check, err := filepath.Abs("scripts/check-running.ps1")
	if err != nil {
		return "", err
	}
	test := ""
	if isolated {
		test = "!define TEXTVIEWER_ISOLATED_TEST\n"
	}
	return strings.NewReplacer(
		"@NAME@", nsisQuote(config.Name),
		"@DIRECTORY@", nsisEscape(config.Name),
		"@IDENTIFIER@", nsisEscape(config.Identifier),
		"@VERSION@", nsisQuote(config.Version),
		"@VERSION4@", nsisQuote(config.Version+".0"),
		"@OUTPUT@", nsisQuote(target),
		"@ICON@", nsisQuote(icon),
		"@AMD64@", payload[0],
		"@ARM64@", payload[1],
		"@HOOKS@", nsisQuote(hooks),
		"@CHECK@", nsisQuote(check),
		"@TEST@", test,
	).Replace(installerTemplate), nil
}

func nsisEscape(value string) string {
	return strings.NewReplacer("$", "$$", "\"", "$\\\"", "\r", "$\\r", "\n", "$\\n").Replace(value)
}

func nsisQuote(value string) string { return `"` + nsisEscape(value) + `"` }

func pngIcon(png []byte) []byte {
	var data bytes.Buffer
	binary.Write(&data, binary.LittleEndian, [3]uint16{0, 1, 1})
	data.Write([]byte{0, 0, 0, 0})
	binary.Write(&data, binary.LittleEndian, [2]uint16{1, 32})
	binary.Write(&data, binary.LittleEndian, [2]uint32{uint32(len(png)), 22})
	data.Write(png)
	return data.Bytes()
}

func findNSIS() (string, error) {
	if override := os.Getenv("MAKENSIS"); override != "" {
		return override, nil
	}
	if tool, err := exec.LookPath("makensis"); err == nil {
		return tool, nil
	}
	for _, base := range []string{os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles")} {
		if base != "" {
			path := filepath.Join(base, "NSIS", "makensis.exe")
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
		}
	}
	if cache, err := os.UserCacheDir(); err == nil {
		dirs, _ := filepath.Glob(filepath.Join(cache, "mygo", "nsis-*"))
		slices.Reverse(dirs)
		for _, dir := range dirs {
			for _, name := range []string{"makensis.exe", filepath.Join("Bin", "makensis.exe")} {
				path := filepath.Join(dir, name)
				if _, err := os.Stat(path); err == nil {
					return path, nil
				}
			}
		}
	}
	return "", fmt.Errorf("NSIS is missing; build a MyGo Windows payload first or set MAKENSIS")
}
