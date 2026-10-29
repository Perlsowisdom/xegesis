// Package passage turns the ways a command can be given Bible verses into one range in English
// numbering, which is how highlights, lists and notes keep verses so that they show in every Bible.
package passage

import (
	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/verse"
)

// Resolve reads verses given as a reference ("John 3:16") or as first and last verse IDs, in the
// numbering named by system (English when empty), and returns them in English numbering. The
// boolean is false when no verses were given.
func Resolve(cmd, ref string, start, end int, system string) (verse.Range, bool, error) {
	var r verse.Range
	switch {
	case ref != "" && (start != 0 || end != 0):
		return r, false, command.Errorf("bad_params", "%s: give ref or verse_start and verse_end, not both", cmd)
	case ref != "":
		var err error
		if r, err = verse.Parse(ref); err != nil {
			return r, false, command.Errorf("bad_params", "%v", err)
		}
	case start != 0:
		if end == 0 {
			end = start
		}
		r = verse.Range{Start: verse.ID(start), End: verse.ID(end)}
		if !r.Start.Valid() || !r.End.Valid() || r.End < r.Start {
			return r, false, command.Errorf("bad_params", "%s: verse_start and verse_end are not a valid range", cmd)
		}
	default:
		return r, false, nil
	}
	if system != "" && verse.System(system) != verse.English {
		c, err := verse.ConvertRange(r, verse.System(system), verse.English)
		if err != nil {
			return r, false, command.Errorf("bad_params", "%s: %v", cmd, err)
		}
		r = c
	}
	return r, true, nil
}
