package main

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer_FlagsWritesThroughFileNodePointer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), Analyzer, "writes")
}
