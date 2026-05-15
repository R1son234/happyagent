package career

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"happyagent/internal/terminal"
)

func handleExportCommand(output io.Writer, workspace *Workspace, input string) error {
	kind := strings.TrimSpace(strings.TrimPrefix(input, "/export"))
	if kind == "" {
		fmt.Fprintln(output, "assistant> 用法：/export <类型>。支持 review-library、jd-match、resume-review、project-pitch、interview-review、review-material。")
		return nil
	}
	if kind == "review-library" || kind == "interview-library" {
		result, err := workspace.GenerateReviewLibrary(time.Now())
		if err != nil {
			return err
		}
		fmt.Fprintf(output, "assistant> 已刷新可复习资料库：面试资料库首页.md")
		if len(result.Paths) > 0 {
			fmt.Fprintf(output, "；更新 %d 个资料文件", len(result.Paths))
		}
		fmt.Fprintln(output)
		return nil
	}
	title, content, err := RenderWorkspaceArtifact(workspace, kind)
	if err != nil {
		return err
	}
	paths, err := workspace.WriteUserOutput(kind, title, content, nil, time.Now())
	if err != nil {
		return err
	}
	fmt.Fprintf(output, "assistant> 已生成并保存 %s：%s\n", title, paths.LatestMarkdown)
	return nil
}

func handleAddCommand(output io.Writer, lineReader terminal.LineReader, workspace *Workspace, input string) error {
	fields := strings.Fields(input)
	if len(fields) < 2 {
		fmt.Fprintln(output, "assistant> 用法：/add <类型> <内容>。支持 jd、resume、prepare、experiences、my-interviews、record。多行内容可输入 /add <类型> 后粘贴，单独一行 . 结束。")
		return nil
	}
	itemType := normalizeWorkspaceType(fields[1])
	inline := strings.TrimSpace(strings.TrimPrefix(input, strings.Join(fields[:2], " ")))
	if !IsSupportedWorkspaceType(itemType) {
		fmt.Fprintf(output, "assistant> 暂不支持归档类型 %q。可用类型：jd、resume、prepare、experiences、my-interviews、record。\n", fields[1])
		return nil
	}
	content := inline
	if content != "" && isExistingFile(content) {
		fileContent, err := extractDocument(context.Background(), content)
		if err != nil {
			return err
		}
		return saveMaterialFileAndPrint(output, workspace, WorkspaceFileInput{
			ItemType:      itemType,
			Text:          fileContent.Text,
			OriginalPath:  content,
			OriginalName:  filepath.Base(content),
			Now:           time.Now(),
			Extractor:     fileContent.Extractor,
			MIMEType:      fileContent.MIMEType,
			ExtractStatus: fileContent.ExtractStatus,
			ExtractError:  fileContent.ExtractError,
		}, "已从文件添加 "+displayWorkspaceType(itemType))
	}
	if content == "" {
		fmt.Fprintf(output, "assistant> 请粘贴 %s 内容，单独一行 . 结束。\n", displayWorkspaceType(itemType))
		var lines []string
		for {
			line, err := lineReader.ReadLine("")
			if err != nil {
				if err == io.EOF {
					break
				}
				return err
			}
			if strings.TrimSpace(line) == "." {
				break
			}
			lines = append(lines, line)
		}
		content = strings.TrimSpace(strings.Join(lines, "\n"))
	}
	if content == "" {
		fmt.Fprintf(output, "assistant> 没有收到 %s 内容，未写入工作区。\n", displayWorkspaceType(itemType))
		return nil
	}
	return saveMaterialAndPrint(output, workspace, itemType, content, "已添加 "+displayWorkspaceType(itemType))
}

func isExistingFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func saveMaterialAndPrint(output io.Writer, workspace *Workspace, itemType string, content string, prefix string) error {
	if strings.ToLower(strings.TrimSpace(itemType)) == WorkspaceTypeExperiences {
		result, err := workspace.ArchivePublicInterviewExperience(content, time.Now())
		if err != nil {
			return err
		}
		fmt.Fprintf(output, "assistant> %s，并保存到 %s；已同步生成 %d 个可复习资料文件，并记录导入流程 %s。\n", prefix, result.ExperienceItem.Path, len(result.GeneratedPaths), result.RecordRel)
		return nil
	}
	guide, err := workspace.LoadGuide()
	if err != nil {
		return err
	}
	classification := ClassifyInputWithGuide(content, guide)
	classification.Type = itemType
	classification.ShouldSave = true
	classification.Reason = "explicit add command"
	classification.RulePath = classificationRulePath(guide, itemType)
	result, err := workspace.AddGuidedMaterial(GuidedMaterialInput{
		ItemType:       itemType,
		Classification: classification,
		Content:        content,
		SourceLabel:    "/add " + itemType,
		Now:            time.Now(),
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(output, "assistant> %s，并保存到 %s；分类记录写入 %s。我已经更新工作区索引，可以基于这些资料做匹配分析、简历优化和面试准备。\n", prefix, result.Item.Path, result.RecordRel)
	return nil
}

func saveMaterialFileAndPrint(output io.Writer, workspace *Workspace, input WorkspaceFileInput, prefix string) error {
	guide, err := workspace.LoadGuide()
	if err != nil {
		return err
	}
	classification := ClassifyInputWithSignals(input.Text, guide, input.OriginalName, input.ItemType, "")
	result, err := workspace.AddGuidedMaterial(GuidedMaterialInput{
		ItemType:       input.ItemType,
		Classification: classification,
		SourceLabel:    input.OriginalPath,
		Now:            input.Now,
		File:           input,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(output, "assistant> %s，并保存到 %s；分类记录写入 %s。我已经更新工作区索引，可以基于这些资料做匹配分析、简历优化和面试准备。\n", prefix, result.Item.Path, result.RecordRel)
	return nil
}

func autoArchiveReferencedFiles(ctx context.Context, output io.Writer, workspace *Workspace, input string) ([]WorkspaceItem, []string, error) {
	paths := extractReferencedFiles(input)
	explicitPaths := make(map[string]bool, len(paths))
	for _, path := range paths {
		explicitPaths[path] = true
	}
	seenPaths := make(map[string]bool, len(paths))
	for _, path := range paths {
		seenPaths[path] = true
	}
	for _, path := range discoverFilesInReferencedDirectories(input) {
		if !seenPaths[path] {
			paths = append(paths, path)
			seenPaths[path] = true
		}
	}
	if len(paths) == 0 {
		return nil, nil, nil
	}
	archived := make([]WorkspaceItem, 0, len(paths))
	ingestErrors := make([]string, 0)
	for _, path := range paths {
		hintType := ""
		if explicitPaths[path] {
			hintType = detectWorkspaceTypeHintNearPath(input, path)
		}
		result, err := IngestFile(ctx, workspace, IngestRequest{
			Path:      path,
			HintType:  hintType,
			UserInput: input,
			Now:       time.Now(),
		})
		if err != nil {
			ingestErrors = append(ingestErrors, fmt.Sprintf("%s: %s", path, err.Error()))
			if result.Item.ID != "" {
				fmt.Fprintf(output, "assistant> 已保存源文件，但提取失败：%s\n", err.Error())
			} else {
				fmt.Fprintf(output, "assistant> 无法自动归档 %s：%s\n", path, err.Error())
			}
		} else {
			fmt.Fprintf(output, "assistant> 已自动归档 %s：%s -> %s\n", displayWorkspaceType(result.ItemType), path, result.Item.Path)
		}
		if result.Item.ID != "" {
			archived = append(archived, result.Item)
		}
	}
	return archived, ingestErrors, nil
}

func printIngestSummary(output io.Writer, workspace *Workspace, items []WorkspaceItem, warnings []string) error {
	if len(items) == 0 && len(warnings) == 0 {
		fmt.Fprintf(output, "assistant> 还没有发现新的可归档资料。请把你准备好的内容放到 %s。\n", filepath.ToSlash(filepath.Join(workspace.Root, "inbox")))
		return nil
	}
	if len(items) > 0 {
		fmt.Fprintln(output, "assistant> 已整理这些资料：")
		for _, item := range items {
			fmt.Fprintf(output, "  - %s：%s\n", displayWorkspaceType(item.Type), item.Path)
		}
	}
	for _, warning := range warnings {
		fmt.Fprintf(output, "assistant> 注意：%s\n", warning)
	}
	meta, _, err := workspace.Status()
	if err != nil {
		return err
	}
	if meta.CurrentResume != "" && meta.ActiveJD != "" {
		fmt.Fprintln(output, "assistant> 当前简历和 JD 都已就绪。你可以直接说：帮我分析一下匹配度。")
		return nil
	}
	fmt.Fprintf(output, "assistant> 请继续把你准备好的内容放到 %s。\n", filepath.ToSlash(filepath.Join(workspace.Root, "inbox")))
	return nil
}

func printCompletionSummary(output io.Writer, title string, inputs []string, paths UserOutputPaths) {
	fmt.Fprintf(output, "assistant> 完成：%s\n", title)
	for _, input := range inputs {
		fmt.Fprintf(output, "assistant> 读取：%s\n", input)
	}
	if paths.LatestMarkdown != "" {
		fmt.Fprintf(output, "assistant> 结果：%s\n", paths.LatestMarkdown)
	}
	if paths.LatestJSON != "" {
		fmt.Fprintf(output, "assistant> JSON：%s\n", paths.LatestJSON)
	}
}
