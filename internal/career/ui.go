package career

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func printCareerWelcome(output io.Writer, workspace *Workspace, sessionID string) {
	meta, index, err := workspace.Status()
	if err != nil {
		fmt.Fprintf(output, "求职助手 Career Copilot\n\n工作区：%s\n会话：%s\n", workspace.Root, sessionID)
		return
	}
	counts := workspaceCounts(index)
	fmt.Fprintln(output, careerBanner())
	fmt.Fprintln(output, "求职助手 Career Copilot")
	fmt.Fprintln(output)
	box := renderCareerBox("求职工作台", []string{
		"请把你准备好的内容放到：",
		"",
		"  " + filepath.ToSlash(filepath.Join(workspace.Root, "inbox")) + "/",
		"",
		"支持内容：简历、JD、项目经历、面经、面试记录、复习笔记",
	}, "当前资料", []string{
		"简历：" + displayPointer(meta.CurrentResume, "未发现"),
		"JD：" + displayPointer(meta.ActiveJD, "未发现"),
		fmt.Sprintf("项目素材：%d 份", counts[WorkspaceTypePrepare]),
		fmt.Sprintf("面经/面试记录：%d 份", counts[WorkspaceTypeExperiences]+counts[WorkspaceTypeMyInterviews]),
	}, "生成结果", []string{
		"复习入口：",
		"",
		"  " + filepath.ToSlash(filepath.Join(workspace.Root, "面试资料库首页.md")),
		"",
		"报告和建议会保存到：",
		"",
		"  " + filepath.ToSlash(filepath.Join(workspace.Root, "outputs")) + "/",
	}, "你可以直接说", []string{
		"我把简历和 JD 放进 inbox 了，帮我分析一下",
		"帮我针对当前岗位优化简历",
		"帮我生成面试准备材料",
		"我刚面完，帮我复盘一下",
	})
	fmt.Fprintln(output, box)
	fmt.Fprintf(output, "会话：%s\n", sessionID)
}

func printCareerHelp(output io.Writer) {
	fmt.Fprintln(output, "你可以直接这样说：")
	fmt.Fprintln(output, "  我把简历和 JD 放进 inbox 了，帮我分析一下")
	fmt.Fprintln(output, "  帮我针对当前岗位优化简历")
	fmt.Fprintln(output, "  帮我生成面试准备材料")
	fmt.Fprintln(output, "  我刚面完，帮我复盘一下")
	fmt.Fprintln(output)
	fmt.Fprintln(output, "高级命令：")
	fmt.Fprintln(output, "  /help     查看帮助")
	fmt.Fprintln(output, "  /status   查看当前工作区状态")
	fmt.Fprintln(output, "  /library  刷新可复习资料库首页、总览、资料包和题库")
	fmt.Fprintln(output, "  /export   生成 review-library、jd-match、resume-review、project-pitch、interview-review、review-material")
	fmt.Fprintln(output, "  /add jd   添加 JD；多行内容用单独一行 . 结束")
	fmt.Fprintln(output, "  /add resume | prepare | experiences | my-interviews | record")
	fmt.Fprintln(output, "  /exit     退出")
}

func isCommandHelpQuestion(input string) bool {
	normalized := strings.ToLower(strings.TrimSpace(input))
	if normalized == "" {
		return false
	}
	if strings.Contains(normalized, "command") || strings.Contains(normalized, "commands") {
		return strings.Contains(normalized, "available") || strings.Contains(normalized, "what") || strings.Contains(normalized, "list") || strings.Contains(normalized, "help")
	}
	if strings.Contains(normalized, "命令") {
		return strings.Contains(normalized, "可用") ||
			strings.Contains(normalized, "哪些") ||
			strings.Contains(normalized, "什么") ||
			strings.Contains(normalized, "帮助") ||
			strings.Contains(normalized, "help")
	}
	return strings.Contains(normalized, "有哪些命令") ||
		strings.Contains(normalized, "可用命令") ||
		strings.Contains(normalized, "命令有哪些") ||
		strings.Contains(normalized, "能用什么命令")
}

func printWorkspaceStatus(output io.Writer, workspace *Workspace) error {
	meta, index, err := workspace.Status()
	if err != nil {
		return err
	}
	counts := workspaceCounts(index)
	reportPath := outputPathIfExists(workspace, workspace.LatestOutputPath("report", ".md"))
	if reportPath == "" {
		reportPath = "未生成"
	}
	ready := "可直接生成完整匹配报告"
	if meta.CurrentResume == "" || meta.ActiveJD == "" {
		ready = "还不能生成完整匹配报告"
	}
	box := renderCareerBox("工作区状态", []string{
		"工作区：" + workspace.Root,
		"当前简历：" + displayPointer(meta.CurrentResume, "未发现"),
		"当前 JD：" + displayPointer(meta.ActiveJD, "未发现"),
		"当前项目：" + displayPointer(meta.ActiveProject, "未发现"),
	}, "资料统计", []string{
		fmt.Sprintf("简历：%d 份", counts[WorkspaceTypeResume]),
		fmt.Sprintf("JD：%d 份", counts[WorkspaceTypeJD]),
		fmt.Sprintf("项目素材：%d 份", counts[WorkspaceTypePrepare]),
		fmt.Sprintf("面经：%d 份", counts[WorkspaceTypeExperiences]),
		fmt.Sprintf("面试记录：%d 份", counts[WorkspaceTypeMyInterviews]),
		fmt.Sprintf("记录：%d 份", counts[WorkspaceTypeRecord]),
	}, "生成结果", []string{
		"最新报告：" + reportPath,
		"输出目录：" + filepath.ToSlash(filepath.Join(workspace.Root, "outputs")) + "/",
	}, "就绪情况", []string{
		ready,
		missingMaterialsHint(workspace, meta),
	})
	fmt.Fprintln(output, box)
	return nil
}

func workspaceCounts(index WorkspaceIndex) map[string]int {
	counts := map[string]int{}
	for _, item := range index.Items {
		counts[item.Type]++
	}
	return counts
}

func displayPointer(path string, fallback string) string {
	if strings.TrimSpace(path) == "" {
		return fallback
	}
	return path
}

func outputPathIfExists(workspace *Workspace, rel string) string {
	if strings.TrimSpace(rel) == "" {
		return ""
	}
	if _, err := os.Stat(filepath.Join(workspace.Root, filepath.FromSlash(rel))); err != nil {
		return ""
	}
	return rel
}

func missingMaterialsHint(workspace *Workspace, meta WorkspaceMetadata) string {
	var missing []string
	if meta.CurrentResume == "" {
		missing = append(missing, "简历")
	}
	if meta.ActiveJD == "" {
		missing = append(missing, "JD")
	}
	if len(missing) == 0 {
		return "你可以直接说：帮我分析一下匹配度"
	}
	return "缺少：" + strings.Join(missing, "、") + "。请放到 " + filepath.ToSlash(filepath.Join(workspace.Root, "inbox")) + "/"
}

func careerBanner() string {
	return strings.TrimRight(`
██╗  ██╗ █████╗ ██████╗ ██████╗ ██╗   ██╗
██║  ██║██╔══██╗██╔══██╗██╔══██╗╚██╗ ██╔╝
███████║███████║██████╔╝██████╔╝ ╚████╔╝
██╔══██║██╔══██║██╔═══╝ ██╔═══╝   ╚██╔╝
██║  ██║██║  ██║██║     ██║        ██║
╚═╝  ╚═╝╚═╝  ╚═╝╚═╝     ╚═╝        ╚═╝
`, "\n")
}

func renderCareerBox(title string, intro []string, section1 string, body1 []string, section2 string, body2 []string, section3 string, body3 []string) string {
	const innerWidth = 64
	var lines []string
	lines = append(lines, boxTop(title, innerWidth))
	for _, line := range intro {
		lines = append(lines, boxLine(line, innerWidth))
	}
	lines = append(lines, boxSection(section1, innerWidth))
	for _, line := range body1 {
		lines = append(lines, boxLine(line, innerWidth))
	}
	lines = append(lines, boxSection(section2, innerWidth))
	for _, line := range body2 {
		lines = append(lines, boxLine(line, innerWidth))
	}
	lines = append(lines, boxSection(section3, innerWidth))
	for _, line := range body3 {
		lines = append(lines, boxLine(line, innerWidth))
	}
	lines = append(lines, boxBottom(innerWidth))
	return strings.Join(lines, "\n")
}

func boxTop(title string, width int) string {
	return "╭" + centeredRule(title, width) + "╮"
}

func boxSection(title string, width int) string {
	return "├" + centeredRule(title, width) + "┤"
}

func boxBottom(width int) string {
	return "╰" + strings.Repeat("─", width+2) + "╯"
}

func centeredRule(title string, width int) string {
	label := " " + title + " "
	remaining := width + 2 - displayWidth(label)
	if remaining < 0 {
		remaining = 0
	}
	left := remaining / 2
	right := remaining - left
	return strings.Repeat("─", left) + label + strings.Repeat("─", right)
}

func boxLine(content string, width int) string {
	padding := width - displayWidth(content)
	if padding < 0 {
		padding = 0
	}
	return "│ " + content + strings.Repeat(" ", padding) + " │"
}

func displayWidth(value string) int {
	width := 0
	for _, r := range value {
		width += runeDisplayWidth(r)
	}
	return width
}

func runeDisplayWidth(r rune) int {
	switch {
	case r == 0:
		return 0
	case r < 0x20 || (r >= 0x7f && r < 0xa0):
		return 0
	case r >= 0x1100 && r <= 0x115f:
		return 2
	case r >= 0x2e80 && r <= 0xa4cf:
		return 2
	case r >= 0xac00 && r <= 0xd7a3:
		return 2
	case r >= 0xf900 && r <= 0xfaff:
		return 2
	case r >= 0xfe10 && r <= 0xfe19:
		return 2
	case r >= 0xfe30 && r <= 0xfe6f:
		return 2
	case r >= 0xff00 && r <= 0xff60:
		return 2
	case r >= 0xffe0 && r <= 0xffe6:
		return 2
	default:
		return 1
	}
}
