package cli

import (
	"fmt"
	"io"
	"strings"
)

// tablePrinter 简单文本表格：按列内容最大宽度对齐，支持中文安全统计。
type tablePrinter struct {
	w     io.Writer
	rows  [][]string
	width []int
}

// newTablePrinter 创建表格输出器。
func newTablePrinter(w io.Writer) *tablePrinter {
	return &tablePrinter{w: w}
}

// addRow 添加一行（列数由首行决定；不足补空，超出截断到表头列数）。
func (t *tablePrinter) addRow(cells ...string) {
	if len(t.width) == 0 {
		t.width = make([]int, len(cells))
	}
	// 实际展示列数以表头为准；后续行超出列数时忽略多余单元格
	cols := len(t.width)
	for i := 0; i < cols; i++ {
		var cell string
		if i < len(cells) {
			cell = cells[i]
		}
		if t.runeLen(cell) > t.width[i] {
			t.width[i] = t.runeLen(cell)
		}
	}
	row := make([]string, cols)
	copy(row, cells)
	t.rows = append(t.rows, row)
}

// padding 补足左侧空白直到目标显示宽度。
func (t *tablePrinter) pad(s string, width int) string {
	n := width - t.runeLen(s)
	if n <= 0 {
		return s
	}
	return s + strings.Repeat(" ", n)
}

// runeLen 字符串显示宽度（按字符数，中文按 1 计）。
func (t *tablePrinter) runeLen(s string) int {
	return len([]rune(s))
}

// print 输出表格：各列用两个空格分隔。
func (t *tablePrinter) print() {
	if len(t.rows) == 0 {
		return
	}
	for _, row := range t.rows {
		line := make([]string, len(row))
		for i, cell := range row {
			if i == len(row)-1 {
				line[i] = cell
			} else {
				line[i] = t.pad(cell, t.width[i])
			}
		}
		fmt.Fprintln(t.w, strings.Join(line, "  "))
	}
}
