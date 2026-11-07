package main

import (
	"os"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
)

// startProfiling honours PHO_CPUPROFILE, PHO_MEMPROFILE and PHO_TRACE (file
// paths) so a real session can be profiled; the returned func flushes them.
func startProfiling(warn func(string, ...any)) func() {
	var stops []func()
	if path := os.Getenv("PHO_CPUPROFILE"); path != "" {
		if f, err := os.Create(path); err != nil {
			warn("cpu profile", "err", err)
		} else if err := pprof.StartCPUProfile(f); err != nil {
			warn("cpu profile", "err", err)
			_ = f.Close()
		} else {
			stops = append(stops, func() { pprof.StopCPUProfile(); _ = f.Close() })
		}
	}
	if path := os.Getenv("PHO_TRACE"); path != "" {
		if f, err := os.Create(path); err != nil {
			warn("trace", "err", err)
		} else if err := trace.Start(f); err != nil {
			warn("trace", "err", err)
			_ = f.Close()
		} else {
			stops = append(stops, func() { trace.Stop(); _ = f.Close() })
		}
	}
	if path := os.Getenv("PHO_MEMPROFILE"); path != "" {
		stops = append(stops, func() {
			f, err := os.Create(path)
			if err != nil {
				warn("mem profile", "err", err)
				return
			}
			defer f.Close()
			runtime.GC()
			if err := pprof.WriteHeapProfile(f); err != nil {
				warn("mem profile", "err", err)
			}
		})
	}
	return func() {
		for _, stop := range stops {
			stop()
		}
	}
}
