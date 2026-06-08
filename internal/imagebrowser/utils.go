package imagebrowser

import (
	"archive/tar"
	"path"
	"sort"
	"strings"

	"github.com/lwmacct/260607-ociget/internal/ociimage"
)

func normalizeDirectory(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" || input == "/" || input == "." {
		return "", nil
	}
	return ociimage.NormalizePath(input)
}

func normalizeLayerPath(input string) string {
	input = strings.TrimSpace(input)
	input = strings.TrimPrefix(input, "./")
	cleaned := path.Clean("/" + input)
	if cleaned == "/" || cleaned == "/." {
		return ""
	}
	return strings.TrimPrefix(cleaned, "/")
}

func directChild(parent, candidate string) (string, bool) {
	if parent == "" {
		if candidate == "" {
			return "", false
		}
		name := strings.Split(candidate, "/")[0]
		return name, name != ""
	}
	if candidate == parent {
		return "", false
	}
	prefix := parent + "/"
	if !strings.HasPrefix(candidate, prefix) {
		return "", false
	}
	rest := strings.TrimPrefix(candidate, prefix)
	name := strings.Split(rest, "/")[0]
	return name, name != ""
}

func childPath(parent, name string) string {
	if parent == "" {
		return name
	}
	return parent + "/" + name
}

func whiteoutTarget(entry string) (string, bool) {
	base := path.Base(entry)
	if !strings.HasPrefix(base, ".wh.") || base == ".wh..wh..opq" {
		return "", false
	}
	dir := path.Dir(entry)
	target := strings.TrimPrefix(base, ".wh.")
	if dir == "." {
		return target, true
	}
	return dir + "/" + target, true
}

func opaqueWhiteoutDir(entry string) (string, bool) {
	if !strings.HasSuffix(entry, "/.wh..wh..opq") && entry != ".wh..wh..opq" {
		return "", false
	}
	dir := strings.TrimSuffix(entry, "/.wh..wh..opq")
	if dir == ".wh..wh..opq" {
		return "", true
	}
	return dir, true
}

func entryType(header *tar.Header, syntheticDir bool) EntryType {
	if syntheticDir {
		return EntryTypeDirectory
	}
	switch header.Typeflag {
	case tar.TypeDir:
		return EntryTypeDirectory
	case tar.TypeReg, tar.TypeRegA:
		return EntryTypeFile
	case tar.TypeSymlink, tar.TypeLink:
		return EntryTypeSymlink
	default:
		return EntryTypeOther
	}
}

func sortedEntries(entries map[string]Entry) []Entry {
	out := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type == EntryTypeDirectory && out[j].Type != EntryTypeDirectory {
			return true
		}
		if out[i].Type != EntryTypeDirectory && out[j].Type == EntryTypeDirectory {
			return false
		}
		return out[i].Name < out[j].Name
	})
	return out
}
