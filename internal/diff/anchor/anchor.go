package anchor

import (
	"github.com/utkarsh261/pho/internal/diff/model"
)

func Generate(dm *model.DiffModel, commitSHA string) {
	if dm == nil || commitSHA == "" {
		return
	}

	for fi := range dm.Files {
		file := &dm.Files[fi]
		path := file.NewPath
		if file.Status == "removed" {
			path = file.OldPath
		}

		for hi := range file.Hunks {
			hunk := &file.Hunks[hi]
			for li := range hunk.Lines {
				line := &hunk.Lines[li]

				switch line.Kind {
				case "context", "addition":
					ptr := line.NewLine
					if ptr == nil {
						continue
					}
					lineNum := *ptr
					line.Anchors = []model.LineAnchor{
						{
							Path:      path,
							CommitSHA: commitSHA,
							Side:      "RIGHT",
							Line:      &lineNum,
						},
					}
				case "deletion":
					ptr := line.OldLine
					if ptr == nil {
						continue
					}
					lineNum := *ptr
					line.Anchors = []model.LineAnchor{
						{
							Path:      path,
							CommitSHA: commitSHA,
							Side:      "LEFT",
							Line:      &lineNum,
						},
					}
					// "hunk_header", "file_header" → no anchor
				}
			}
		}
	}
}
