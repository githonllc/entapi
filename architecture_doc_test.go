package entapi

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestArchitectureDocumentCountsTrackRepository(t *testing.T) {
	templatePaths, err := fs.Glob(templateFS, "templates/*.tmpl")
	if err != nil {
		t.Fatalf("listing embedded templates: %v", err)
	}
	if len(templatePaths) == 0 {
		t.Fatal("embedded template count is zero; refusing to let the documentation guard pass vacuously")
	}

	root := repoRoot(t)
	nestedModules := 0
	err = filepath.WalkDir(filepath.Join(root, "internal"), func(_ string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && entry.Name() == "go.mod" {
			nestedModules++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("counting nested modules under internal/: %v", err)
	}
	if nestedModules == 0 {
		t.Fatal("nested module count is zero; refusing to let the documentation guard pass vacuously")
	}

	const docPath = "docs/ARCHITECTURE.md"
	doc, err := os.ReadFile(filepath.Join(root, docPath))
	if err != nil {
		t.Fatalf("reading %s: %v", docPath, err)
	}
	lines := strings.Split(string(doc), "\n")

	// Prose is not a stable data format. These three rows already carry the
	// counts as digits, so parse those digits and fail if a row disappears
	// instead of inventing a second metadata convention that can drift too.
	packageCounts := architectureDocMatch(t, docPath, lines,
		regexp.MustCompile(`^\| 包数 \|.*\+ ([0-9]+) 个模板 \+ ([0-9]+) 个嵌套模块 \|`), "包数")
	diagramTemplates := architectureDocMatch(t, docPath, lines,
		regexp.MustCompile(`component "templates/\*\.tmpl\\n\(([0-9]+) 个, go:embed\)" as TMPL`), "架构图模板节点")
	moduleTemplates := architectureDocMatch(t, docPath, lines,
		regexp.MustCompile(`^\| 模板 \|.*（([0-9]+) 个）`), "模块表模板行")

	assertArchitectureDocCount(t, docPath, "“包数”行", packageCounts[1], len(templatePaths), "模板")
	assertArchitectureDocCount(t, docPath, "架构图模板节点", diagramTemplates[1], len(templatePaths), "模板")
	assertArchitectureDocCount(t, docPath, "模块表模板行", moduleTemplates[1], len(templatePaths), "模板")
	assertArchitectureDocCount(t, docPath, "“包数”行", packageCounts[2], nestedModules, "嵌套模块")
}

func architectureDocMatch(t *testing.T, docPath string, lines []string, pattern *regexp.Regexp, label string) []string {
	t.Helper()
	for _, line := range lines {
		if match := pattern.FindStringSubmatch(line); match != nil {
			return match
		}
	}
	t.Fatalf("%s 缺少可解析的%s；请恢复该行中的计数数字，让架构文档漂移守卫能够检查它", docPath, label)
	return nil
}

func assertArchitectureDocCount(t *testing.T, docPath, label, documented string, actual int, unit string) {
	t.Helper()
	stale, err := strconv.Atoi(documented)
	if err != nil {
		t.Fatalf("%s 的%s含有无法解析的计数 %q；请改成十进制数字", docPath, label, documented)
	}
	if stale != actual {
		t.Errorf("%s 的%s写的是 %d 个%s，源码树实际是 %d 个%s；请更新 %s", docPath, label, stale, unit, actual, unit, docPath)
	}
}
