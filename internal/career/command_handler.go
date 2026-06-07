package career

import (
	"fmt"
	"io"
	"path/filepath"
)

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
