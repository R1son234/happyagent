package runtime

import (
	"context"
	"fmt"
	"time"

	"happyagent/internal/config"
	"happyagent/internal/engine"
	"happyagent/internal/llm"
	"happyagent/internal/mcp"
	"happyagent/internal/skills"
	"happyagent/internal/tools"
)

type Builder struct{}

func NewBuilder() *Builder {
	return &Builder{}
}

func (b *Builder) Build(cfg config.Config) (*Runtime, error) {
	_ = b

	client, err := llm.NewClient(cfg.LLM)
	if err != nil {
		return nil, err
	}

	registry := tools.NewRegistry()
	defs, err := registerBuiltinTools(registry, cfg.Tools)
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

		readResourceTool := tools.NewMCPReadResourceTool(manager)
		registry.MustRegister(readResourceTool)
		defs = append(defs, readResourceTool.Definition())
	}

	skillLoader := skills.NewLoader(cfg.Skills.Dir)
	rt := &Runtime{
		tools:               defs,
		maxObservationBytes: cfg.Engine.MaxObservationBytes,
		mcpManager:          manager,
		skillLoader:         skillLoader,
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

func registerBuiltinTools(registry *tools.Registry, cfg config.ToolsConfig) ([]tools.Definition, error) {
	var registered []tools.Definition

	finalAnswer := tools.NewFinalAnswerTool()
	registry.MustRegister(finalAnswer)
	registered = append(registered, finalAnswer.Definition())

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

	fileList, err := tools.NewFileListTool(cfg.RootDir)
	if err != nil {
		return nil, err
	}
	registry.MustRegister(fileList)
	registered = append(registered, fileList.Definition())

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
