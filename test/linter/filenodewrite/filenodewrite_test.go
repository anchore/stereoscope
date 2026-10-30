package main

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer_FlagsWritesThroughFileNodePointer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), Analyzer, "writes")
}

func TestAnalyzer_FlagsWriteThroughParenthesizedDeref(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), Analyzer, "parenderef")
}

func TestAnalyzer_FlagsWriteThroughEmbeddedFileNode(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), Analyzer, "embedded")
}

func TestAnalyzer_HonorsNolint(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), Analyzer, "nolint")
}

func TestAnalyzer_MessageNamesByValueCopy(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), Analyzer, "message")
}
