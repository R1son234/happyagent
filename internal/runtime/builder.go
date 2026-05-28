package runtime

import (
	"context"
	"fmt"
	"time"

	"happyagent/internal/agents"
	"happyagent/internal/background"
	"happyagent/internal/config"
	"happyagent/internal/engine"
	"happyagent/internal/llm"
	"happyagent/internal/mcp"
	"happyagent/internal/memory"
	"happyagent/internal/skills"
	"happyagent/internal/tasks"
	"happyagent/internal/tools"
	"happyagent/internal/worktree"
)

const defaultProfileDir = "profiles"

type Builder struct {
	profileDir string
}

func NewBuilder() *Builder {
	return &Builder{
		profileDir: defaultProfileDir,
	}
}

func (b *Builder) WithProfileDir(dir string) *Builder {
	b.profileDir = dir
	return b
}

func (b *Builder) Build(cfg config.Config) (*Runtime, error) {
	client, err := llm.NewClient(cfg.LLM)
	if err != nil {
		return nil, err
	}

	registry := tools.NewRegistry()
	defs, err := registerBuiltinTools(registry, cfg.Tools, cfg.Web)
	if err != nil {
		return nil, err
	}

	var manager *mcp.Manager
	if len(cfg.MCP.Servers) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.MCP.ConnectTimeoutSeconds)*time.Second)
		defer cancel()

		manager, err = mcp.NewManager(ctx, cfg.MCP)
		if err != nil {
			return nil, err
		}

		mcpDefs, err := manager.RegisterTools(registry)
		if err != nil {
			manager.Close()
			return nil, err
		}
		defs = append(defs, mcpDefs...)

		if err := manager.RegisterPromptTool(registry); err != nil {
			manager.Close()
			return nil, err
		}

		readResourceTool := tools.NewMCPReadResourceTool(manager)
		registry.MustRegister(readResourceTool)
		defs = append(defs, readResourceTool.Definition())
	}

	skillLoader := skills.NewLoader(cfg.Skills.Dir)

	memStore := memory.NewLongTermStore(".happyagent/memory")
	taskStore, err := tasks.NewStore(cfg.Tools.RootDir)
	if err != nil {
		return nil, err
	}
	agentStore, err := agents.NewStore(cfg.Tools.RootDir)
	if err != nil {
		return nil, err
	}
	worktreeManager, err := worktree.NewManager(cfg.Tools.RootDir)
	if err != nil {
		return nil, err
	}
	memSave := tools.NewMemorySaveTool(memStore)
	registry.MustRegister(memSave)
	defs = append(defs, memSave.Definition())
	memDelete := tools.NewMemoryDeleteTool(memStore)
	registry.MustRegister(memDelete)
	defs = append(defs, memDelete.Definition())
	memRecall := tools.NewMemoryRecallTool(memStore)
	registry.MustRegister(memRecall)
	defs = append(defs, memRecall.Definition())

	rt := &Runtime{
		tools:               defs,
		maxObservationBytes: cfg.Engine.MaxObservationBytes,
		offload: engine.OffloadConfig{
			Enabled:  cfg.Engine.OffloadEnabled,
			MinBytes: cfg.Engine.OffloadMinBytes,
			Dir:      cfg.Engine.OffloadDir,
			RootDir:  cfg.Tools.RootDir,
		},
		mcpManager:      manager,
		skillLoader:     skillLoader,
		profileDir:      b.profileDir,
		memoryStore:     memStore,
		taskStore:       taskStore,
		agentStore:      agentStore,
		backgroundStore: background.NewStore(cfg.Tools.RootDir),
		worktreeManager: worktreeManager,
	}
	registry.MustRegister(tools.NewActivateSkillTool(func(ctx context.Context) tools.ActivateSkillProvider {
		return tools.ActivateSkillProviderFromContext(ctx)
	}))
	registry.MustRegister(tools.NewListCapabilitiesTool(func(ctx context.Context) tools.CapabilityProvider {
		return tools.CapabilityProviderFromContext(ctx)
	}))

	rt.runner = engine.NewRunner(client, registry, cfg.Engine.LoopMaxSteps)
	return rt, nil
}

func registerBuiltinTools(registry *tools.Registry, cfg config.ToolsConfig, webCfg config.WebConfig) ([]tools.Definition, error) {
	var registered []tools.Definition

	finalAnswer := tools.NewFinalAnswerTool()
	registry.MustRegister(finalAnswer)
	registered = append(registered, finalAnswer.Definition())

	writeTodos := tools.NewWriteTodosTool()
	registry.MustRegister(writeTodos)
	registered = append(registered, writeTodos.Definition())

	for _, taskTool := range tools.NewTaskTools(func(ctx context.Context) tools.TaskProvider {
		return tools.TaskProviderFromContext(ctx)
	}) {
		registry.MustRegister(taskTool)
		registered = append(registered, taskTool.Definition())
	}

	for _, agentTool := range tools.NewAgentTools(func(ctx context.Context) tools.AgentProvider {
		return tools.AgentProviderFromContext(ctx)
	}) {
		registry.MustRegister(agentTool)
		registered = append(registered, agentTool.Definition())
	}

	for _, worktreeTool := range tools.NewWorktreeTools(func(ctx context.Context) tools.WorktreeProvider {
		return tools.WorktreeProviderFromContext(ctx)
	}) {
		registry.MustRegister(worktreeTool)
		registered = append(registered, worktreeTool.Definition())
	}

	fileRead, err := tools.NewFileReadTool(cfg.RootDir)
	if err != nil {
		return nil, err
	}
	registry.MustRegister(fileRead)
	registered = append(registered, fileRead.Definition())

	fileSearch, err := tools.NewFileSearchTool(cfg.RootDir)
	if err != nil {
		return nil, err
	}
	registry.MustRegister(fileSearch)
	registered = append(registered, fileSearch.Definition())

	searchDocs, err := tools.NewSearchDocsTool(cfg.RootDir)
	if err != nil {
		return nil, err
	}
	registry.MustRegister(searchDocs)
	registered = append(registered, searchDocs.Definition())

	fileList, err := tools.NewFileListTool(cfg.RootDir)
	if err != nil {
		return nil, err
	}
	registry.MustRegister(fileList)
	registered = append(registered, fileList.Definition())

	if webCfg.Enabled {
		webSearch := tools.NewWebSearchTool(webCfg)
		registry.MustRegister(webSearch)
		registered = append(registered, webSearch.Definition())

		webFetch := tools.NewWebFetchTool(webCfg)
		registry.MustRegister(webFetch)
		registered = append(registered, webFetch.Definition())
	}

	if cfg.WriteEnabled {
		filePatch, err := tools.NewFilePatchTool(cfg.RootDir)
		if err != nil {
			return nil, err
		}
		registry.MustRegister(filePatch)
		registered = append(registered, filePatch.Definition())

		fileWrite, err := tools.NewFileWriteTool(cfg.RootDir, cfg.WriteMaxBytes, cfg.WriteRequireOverwrite)
		if err != nil {
			return nil, err
		}
		registry.MustRegister(fileWrite)
		registered = append(registered, fileWrite.Definition())

	}

	if cfg.DeleteEnabled {
		fileDelete, err := tools.NewFileDeleteTool(cfg.RootDir, cfg.DeleteRequireConfirmation)
		if err != nil {
			return nil, err
		}
		registry.MustRegister(fileDelete)
		registered = append(registered, fileDelete.Definition())
	}

	if cfg.ShellEnabled {
		shell, err := tools.NewShellTool(cfg.RootDir, cfg.ShellAllowedCommands)
		if err != nil {
			return nil, err
		}
		registry.MustRegister(shell)
		registered = append(registered, shell.Definition())
	}

	if len(registered) == 0 {
		return nil, fmt.Errorf("no tools enabled")
	}

	return registered, nil
}
