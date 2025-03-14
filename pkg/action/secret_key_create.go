package action

import (
	"context"
	"fmt"
	"os"

	"github.com/werf/common-go/pkg/secrets_manager"
	"github.com/werf/nelm/pkg/log"
)

const (
	DefaultSecretKeyCreateLogLevel = log.ErrorLevel
)

type SecretKeyCreateOptions struct {
	LogColorMode  LogColorMode
	LogLevel      log.Level
	OutputNoPrint bool
	TempDirPath   string
}

func SecretKeyCreate(ctx context.Context, opts SecretKeyCreateOptions) (string, error) {
	if opts.LogLevel != "" {
		log.Default.SetLevel(ctx, opts.LogLevel)
	} else {
		log.Default.SetLevel(ctx, DefaultSecretKeyCreateLogLevel)
	}

	opts, err := applySecretKeyCreateOptionsDefaults(opts)
	if err != nil {
		return "", fmt.Errorf("build secret key create options: %w", err)
	}

	var result string
	if !opts.OutputNoPrint {
		if keyByte, err := secrets_manager.GenerateSecretKey(); err != nil {
			return "", fmt.Errorf("generate secret key: %w", err)
		} else {
			result = string(keyByte)
		}

		fmt.Println(result)
	}

	return result, nil
}

func applySecretKeyCreateOptionsDefaults(opts SecretKeyCreateOptions) (SecretKeyCreateOptions, error) {
	var err error
	if opts.TempDirPath == "" {
		opts.TempDirPath, err = os.MkdirTemp("", "")
		if err != nil {
			return SecretKeyCreateOptions{}, fmt.Errorf("create temp dir: %w", err)
		}
	}

	opts.LogColorMode = applyLogColorModeDefault(opts.LogColorMode, false)

	return opts, nil
}
