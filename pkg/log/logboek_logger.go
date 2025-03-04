package log

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/samber/lo"

	"github.com/werf/kubedog/pkg/trackers/dyntracker/util"
	"github.com/werf/logboek"
	"github.com/werf/logboek/pkg/level"
	"github.com/werf/logboek/pkg/types"
)

var _ Logger = (*LogboekLogger)(nil)

func NewLogboekLogger() *LogboekLogger {
	return &LogboekLogger{
		traceStash: util.NewConcurrent(make(map[string][]string)),
		debugStash: util.NewConcurrent(make(map[string][]string)),
		infoStash:  util.NewConcurrent(make(map[string][]string)),
		warnStash:  util.NewConcurrent(make(map[string][]string)),
		errorStash: util.NewConcurrent(make(map[string][]string)),

		level: util.NewConcurrent(lo.ToPtr(InfoLevel)),
	}
}

type LogboekLogger struct {
	traceStash *util.Concurrent[map[string][]string]
	debugStash *util.Concurrent[map[string][]string]
	infoStash  *util.Concurrent[map[string][]string]
	warnStash  *util.Concurrent[map[string][]string]
	errorStash *util.Concurrent[map[string][]string]

	level *util.Concurrent[*Level]
}

func (l *LogboekLogger) Trace(ctx context.Context, format string, a ...interface{}) {
	if !l.AcceptLevel(nil, TraceLevel) {
		return
	}

	logboek.Context(ctx).Debug().LogF(format+"\n", a...)
}

func (l *LogboekLogger) TraceStruct(ctx context.Context, obj interface{}, format string, a ...interface{}) {
	if !l.AcceptLevel(nil, TraceLevel) {
		return
	}

	out, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		l.Warn(ctx, "error marshaling object to json while tracing struct for %q: %w", fmt.Sprintf(format, a...), err)
	}

	logboek.Context(ctx).Debug().LogF(fmt.Sprintf(format+"\n", a...) + string(out) + "\n")
}

func (l *LogboekLogger) TracePush(ctx context.Context, group string, format string, a ...interface{}) {
	l.traceStash.RWTransaction(func(stash map[string][]string) {
		stash[group] = append(stash[group], fmt.Sprintf(format, a...))
	})
}

func (l *LogboekLogger) TracePop(ctx context.Context, group string) {
	l.traceStash.RWTransaction(func(stash map[string][]string) {
		for _, msg := range stash[group] {
			l.Trace(ctx, msg)
		}

		delete(stash, group)
	})
}

func (l *LogboekLogger) Debug(ctx context.Context, format string, a ...interface{}) {
	if !l.AcceptLevel(nil, DebugLevel) {
		return
	}

	logboek.Context(ctx).Debug().LogF(format+"\n", a...)
}

func (l *LogboekLogger) DebugPush(ctx context.Context, group string, format string, a ...interface{}) {
	l.debugStash.RWTransaction(func(stash map[string][]string) {
		stash[group] = append(stash[group], fmt.Sprintf(format, a...))
	})
}

func (l *LogboekLogger) DebugPop(ctx context.Context, group string) {
	l.debugStash.RWTransaction(func(stash map[string][]string) {
		for _, msg := range stash[group] {
			l.Debug(ctx, msg)
		}

		delete(stash, group)
	})
}

func (l *LogboekLogger) Info(ctx context.Context, format string, a ...interface{}) {
	if !l.AcceptLevel(nil, InfoLevel) {
		return
	}

	logboek.Context(ctx).Default().LogF(format+"\n", a...)
}

func (l *LogboekLogger) InfoPush(ctx context.Context, group string, format string, a ...interface{}) {
	l.infoStash.RWTransaction(func(stash map[string][]string) {
		stash[group] = append(stash[group], fmt.Sprintf(format, a...))
	})
}

func (l *LogboekLogger) InfoPop(ctx context.Context, group string) {
	l.infoStash.RWTransaction(func(stash map[string][]string) {
		for _, msg := range stash[group] {
			l.Info(ctx, msg)
		}

		delete(stash, group)
	})
}

func (l *LogboekLogger) Warn(ctx context.Context, format string, a ...interface{}) {
	if !l.AcceptLevel(nil, WarningLevel) {
		return
	}

	logboek.Context(ctx).Warn().LogFHighlight(format+"\n", a...)
}

func (l *LogboekLogger) WarnPush(ctx context.Context, group string, format string, a ...interface{}) {
	l.warnStash.RWTransaction(func(stash map[string][]string) {
		stash[group] = append(stash[group], fmt.Sprintf(format, a...))
	})
}

func (l *LogboekLogger) WarnPop(ctx context.Context, group string) {
	l.warnStash.RWTransaction(func(stash map[string][]string) {
		for _, msg := range stash[group] {
			l.Warn(ctx, msg)
		}

		delete(stash, group)
	})
}

func (l *LogboekLogger) Error(ctx context.Context, format string, a ...interface{}) {
	if !l.AcceptLevel(nil, ErrorLevel) {
		return
	}

	logboek.Context(ctx).Error().LogFHighlight(format+"\n", a...)
}

func (l *LogboekLogger) ErrorPush(ctx context.Context, group string, format string, a ...interface{}) {
	l.errorStash.RWTransaction(func(stash map[string][]string) {
		stash[group] = append(stash[group], fmt.Sprintf(format, a...))
	})
}

func (l *LogboekLogger) ErrorPop(ctx context.Context, group string) {
	l.errorStash.RWTransaction(func(stash map[string][]string) {
		for _, msg := range stash[group] {
			l.Error(ctx, msg)
		}

		delete(stash, group)
	})
}

func (l *LogboekLogger) InfoBlock(ctx context.Context, format string, a ...interface{}) types.LogBlockInterface {
	return logboek.Context(ctx).Default().LogBlock(format, a...)
}

func (l *LogboekLogger) InfoProcess(ctx context.Context, format string, a ...interface{}) types.LogProcessInterface {
	return logboek.Context(ctx).Default().LogProcess(format, a...)
}

func (l *LogboekLogger) SetLevel(ctx context.Context, lvl Level) {
	switch lvl {
	case DebugLevel, TraceLevel:
		logboek.Context(ctx).SetAcceptedLevel(level.Debug)
	case InfoLevel:
		logboek.Context(ctx).SetAcceptedLevel(level.Default)
	case WarningLevel:
		logboek.Context(ctx).SetAcceptedLevel(level.Warn)
	case ErrorLevel:
		logboek.Context(ctx).SetAcceptedLevel(level.Error)
	case SilentLevel:
		logboek.Context(ctx).Streams().Mute()
	default:
		panic(fmt.Sprintf("unsupported log level %q", lvl))
	}

	l.level.RWTransaction(func(lv *Level) {
		*lv = lvl
	})
}

func (l *LogboekLogger) Level(context.Context) Level {
	var lv Level
	l.level.RTransaction(func(l *Level) {
		lv = *l
	})

	return lv
}

func (l *LogboekLogger) AcceptLevel(ctx context.Context, lvl Level) bool {
	lvlI := slices.Index(Levels, lvl)

	currentLvl := l.Level(ctx)
	currentLvlI := slices.Index(Levels, currentLvl)

	return currentLvlI >= lvlI
}
