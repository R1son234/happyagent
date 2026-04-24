package runtime

import (
	"fmt"

	"happyagent/internal/config"
	"happyagent/internal/engine"
	"happyagent/internal/llm"
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

	return &Runtime{
		runner: engine.NewRunner(client, registry, cfg.Engine.LoopMaxSteps),
		tools:  defs,
	}, nil
}

func registerBuiltinTools(registry *tools.Registry, cfg config.ToolsConfig) ([]tools.Definition, error) {
	var registered []tools.Definition

	fileRead, err := tools.NewFileReadTool(cfg.RootDir)
	if err != nil {
		return nil, err
	}
	registry.MustRegister(fileRead)
	registered = append(registered, fileRead.Definition())

	fileList, err := tools.NewFileListTool(cfg.RootDir)
	if err != nil {
		return nil, err
	}
	registry.MustRegister(fileList)
	registered = append(registered, fileList.Definition())

	if cfg.WriteEnabled {
		fileWrite, err := tools.NewFileWriteTool(cfg.RootDir)
		if err != nil {
			return nil, err
		}
		registry.MustRegister(fileWrite)
		registered = append(registered, fileWrite.Definition())
	}

	if cfg.DeleteEnabled {
		fileDelete, err := tools.NewFileDeleteTool(cfg.RootDir)
		if err != nil {
			return nil, err
		}
		registry.MustRegister(fileDelete)
		registered = append(registered, fileDelete.Definition())
	}

	if cfg.ShellEnabled {
		shell, err := tools.NewShellTool(cfg.RootDir)
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
