package layout

import (
	"fmt"
	"strings"
)

func CalculateLayoutChecksum(layout string) string {
	checksum := 0
	for i := 0; i < len(layout); i++ {
		checksum = (checksum >> 1) + ((checksum & 1) << 15)
		checksum += int(layout[i])
		checksum &= 0xFFFF
	}
	return fmt.Sprintf("%04x", checksum)
}

func GenerateSidebarGridLayout(
	controlPaneID string,
	contentPanes []string,
	sidebarWidth int,
	windowWidth int,
	windowHeight int,
	columns int,
	maxComfortableWidth int,
	isSpacerPane func(paneID string) bool,
) string {
	numContentPanes := len(contentPanes)
	if numContentPanes == 0 || columns <= 0 {
		return ""
	}

	cols := columns
	rows := (numContentPanes + cols - 1) / cols
	contentWidth := windowWidth - sidebarWidth - 1
	contentStartX := sidebarWidth + 1
	bordersHeight := rows - 1
	availableHeight := windowHeight - bordersHeight
	paneHeight := availableHeight / rows

	gridRows := make([]string, 0, rows)
	paneIndex := 0
	currentY := 0

	for row := 0; row < rows; row++ {
		rowPanes := []string{}
		absoluteX := contentStartX
		rowHeight := paneHeight
		if row == rows-1 {
			rowHeight = windowHeight - currentY
		}

		panesInThisRow := []string{}
		for col := 0; col < cols && paneIndex+col < numContentPanes; col++ {
			panesInThisRow = append(panesInThisRow, contentPanes[paneIndex+col])
		}

		rowHasSpacer := false
		if len(panesInThisRow) > 0 && isSpacerPane != nil {
			last := panesInThisRow[len(panesInThisRow)-1]
			rowHasSpacer = isSpacerPane(last) && row == rows-1
		}

		numContentInRow := len(panesInThisRow)
		if rowHasSpacer {
			numContentInRow--
		}

		contentPaneWidths := make([]int, 0, len(panesInThisRow))
		spacerWidth := 0
		if rowHasSpacer {
			bordersInRow := len(panesInThisRow) - 1
			totalContentWidth := numContentInRow * maxComfortableWidth
			spacerWidth = contentWidth - totalContentWidth - bordersInRow
			for i := 0; i < numContentInRow; i++ {
				contentPaneWidths = append(contentPaneWidths, maxComfortableWidth)
			}
		} else {
			bordersInRow := len(panesInThisRow) - 1
			availWidth := contentWidth - bordersInRow
			evenWidth := availWidth / len(panesInThisRow)
			remainder := availWidth - evenWidth*len(panesInThisRow)
			for i := 0; i < len(panesInThisRow); i++ {
				w := evenWidth
				if i == 0 {
					w += remainder
				}
				contentPaneWidths = append(contentPaneWidths, w)
			}
		}

		for col := 0; col < len(panesInThisRow); col++ {
			paneID := strings.TrimPrefix(panesInThisRow[col], "%")
			colWidth := 0
			if rowHasSpacer && col == len(panesInThisRow)-1 {
				colWidth = spacerWidth
			} else {
				colWidth = contentPaneWidths[col]
			}
			rowPanes = append(rowPanes, fmt.Sprintf("%dx%d,%d,%d,%s", colWidth, rowHeight, absoluteX, currentY, paneID))
			absoluteX += colWidth
			if col < len(panesInThisRow)-1 {
				absoluteX++
			}
		}

		paneIndex += len(panesInThisRow)
		if len(rowPanes) > 1 {
			gridRows = append(gridRows, fmt.Sprintf("%dx%d,%d,%d{%s}", contentWidth, rowHeight, contentStartX, currentY, strings.Join(rowPanes, ",")))
		} else if len(rowPanes) == 1 {
			gridRows = append(gridRows, rowPanes[0])
		}

		if row < rows-1 {
			currentY += paneHeight + 1
		}
	}

	sidebarID := strings.TrimPrefix(controlPaneID, "%")
	sidebar := fmt.Sprintf("%dx%d,0,0,%s", sidebarWidth, windowHeight, sidebarID)

	var body string
	if len(gridRows) > 1 {
		contentArea := fmt.Sprintf("%dx%d,%d,0[%s]", contentWidth, windowHeight, contentStartX, strings.Join(gridRows, ","))
		body = fmt.Sprintf("%dx%d,0,0{%s,%s}", windowWidth, windowHeight, sidebar, contentArea)
	} else if len(gridRows) == 1 {
		body = fmt.Sprintf("%dx%d,0,0{%s,%s}", windowWidth, windowHeight, sidebar, gridRows[0])
	} else {
		return ""
	}

	return fmt.Sprintf("%s,%s", CalculateLayoutChecksum(body), body)
}
