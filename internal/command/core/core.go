// Package core registers the foundation commands: app information, verse references, licences,
// notes and change history. Feature packages register their own commands the same way.
package core

import (
	"context"
	"time"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/licence"
	"github.com/Perlsowisdom/xegesis/internal/store"
	"github.com/Perlsowisdom/xegesis/internal/verse"
)

// Version is the Xegesis version, set at build time.
var Version = "0.0.0-dev"

// Register adds the core commands to r.
func Register(r *command.Registry) {
	r.Add(
		command.Define("app.info", "Show the Xegesis version, schema version and device ID.", false, appInfo),
		command.Define("verse.parse", "Read a Bible reference such as \"Rom 8:28-30\" into verse IDs.", false, verseParse),
		command.Define("verse.convert", "Convert a reference between verse-numbering systems.", false, verseConvert),
		command.Define("verse.books", "List the books of the Bible with their numbers and OSIS IDs.", false, verseBooks),
		command.Define("licence.defaults", "Show the starting licence settings for a licence type.", false, licenceDefaults),
		command.Define("licence.check", "Decide whether a licence allows a print, export or share.", false, licenceCheck),
		command.Define("note.create", "Create a note, optionally attached to a Bible reference.", true, noteCreate),
		command.Define("note.get", "Show one note.", false, noteGet),
		command.Define("note.list", "List notes, optionally only those on verses in a reference.", false, noteList),
		command.Define("note.update", "Change a note's title or text.", true, noteUpdate),
		command.Define("note.delete", "Delete a note. It stays in note.deleted until it is restored.", true, noteDelete),
		command.Define("note.search", "Find notes by words in them, tag, book of the Bible or library book, newest first.", false, noteSearch),
		command.Define("note.tags", "List the tags in use with how many notes have each, to suggest tags.", false, noteTags),
		command.Define("note.sources", "List the books of the Bible and library books that have notes, to filter notes by.", false, noteSources),
		command.Define("note.move", "Attach a note to other verses or a book place, detach it so it belongs to none, or file it in a folder.", true, noteMove),
		command.Define("note.deleted", "List deleted notes, most recently deleted first.", false, noteDeleted),
		command.Define("note.restore", "Bring a deleted note back.", true, noteRestore),
		command.Define("history.list", "List the recorded changes to one object, oldest first.", false, historyList),
		command.Define("history.restore", "Return an object to how it was just after a recorded change.", true, historyRestore),
	)
}

func now() int64 { return time.Now().UnixMilli() }

func needStore(env *command.Env) error {
	if env.Store == nil {
		return command.Errorf("unavailable", "no library is open")
	}
	return nil
}

// App ------------------------------------------------------------------------------------------

type AppInfo struct {
	Version       string `json:"version"`
	SchemaVersion int    `json:"schema_version,omitempty"`
	Device        string `json:"device,omitempty"`
}

func appInfo(_ context.Context, env *command.Env, _ struct{}) (AppInfo, error) {
	info := AppInfo{Version: Version}
	if env.Store != nil {
		v, err := env.Store.SchemaVersion()
		if err != nil {
			return info, err
		}
		info.SchemaVersion, info.Device = v, env.Store.Device()
	}
	return info, nil
}

// Verses ---------------------------------------------------------------------------------------

type RefParams struct {
	Ref string `json:"ref" required:"true" desc:"A Bible reference, e.g. \"Rom 8:28-30\" or \"Rom.8.28\""`
}

type RefResult struct {
	Start   verse.ID `json:"start" desc:"First verse ID (BBCCCVVV)"`
	End     verse.ID `json:"end" desc:"Last verse ID (BBCCCVVV); verse 999 means end of chapter"`
	Display string   `json:"display"`
	OSIS    string   `json:"osis"`
}

func refResult(r verse.Range) RefResult {
	osis := r.Start.OSIS()
	if r.End != r.Start {
		osis += "-" + r.End.OSIS()
	}
	return RefResult{Start: r.Start, End: r.End, Display: r.String(), OSIS: osis}
}

func verseParse(_ context.Context, _ *command.Env, p RefParams) (RefResult, error) {
	r, err := verse.Parse(p.Ref)
	if err != nil {
		return RefResult{}, command.Errorf("bad_params", "%v", err)
	}
	return refResult(r), nil
}

type ConvertParams struct {
	Ref  string `json:"ref" required:"true" desc:"A Bible reference"`
	From string `json:"from" required:"true" enum:"english,hebrew" desc:"Numbering the reference uses"`
	To   string `json:"to" required:"true" enum:"english,hebrew" desc:"Numbering to convert to"`
}

func verseConvert(_ context.Context, _ *command.Env, p ConvertParams) (RefResult, error) {
	r, err := verse.Parse(p.Ref)
	if err != nil {
		return RefResult{}, command.Errorf("bad_params", "%v", err)
	}
	c, err := verse.ConvertRange(r, verse.System(p.From), verse.System(p.To))
	if err != nil {
		return RefResult{}, command.Errorf("bad_params", "%v", err)
	}
	return refResult(c), nil
}

func verseBooks(context.Context, *command.Env, struct{}) ([]verse.Book, error) {
	return verse.Books(), nil
}

// Licences -------------------------------------------------------------------------------------

type LicenceDefaultsParams struct {
	Type string `json:"type" required:"true" enum:"public_domain,creative_commons,open_other,personal_purchase,own_work,unknown"`
	SPDX string `json:"spdx,omitempty" desc:"SPDX or Creative Commons ID, e.g. CC-BY-SA-4.0"`
}

func licenceDefaults(_ context.Context, _ *command.Env, p LicenceDefaultsParams) (licence.Licence, error) {
	l, err := licence.Defaults(licence.Type(p.Type), p.SPDX)
	if err != nil {
		return l, command.Errorf("bad_params", "%v", err)
	}
	return l, nil
}

type LicenceCheckParams struct {
	Licence licence.Licence `json:"licence" desc:"The item's licence"`
	Request licence.Request `json:"request" desc:"What the user wants to do"`
}

func licenceCheck(_ context.Context, _ *command.Env, p LicenceCheckParams) (licence.Decision, error) {
	return p.Licence.Check(p.Request), nil
}

// History --------------------------------------------------------------------------------------

type ObjectParams struct {
	ObjectType string `json:"object_type" required:"true" desc:"Table name, e.g. notes"`
	ObjectID   string `json:"object_id" required:"true" desc:"The object's ID"`
}

func historyList(ctx context.Context, env *command.Env, p ObjectParams) ([]store.Change, error) {
	if err := needStore(env); err != nil {
		return nil, err
	}
	h, err := env.Store.History(ctx, p.ObjectType, p.ObjectID)
	if h == nil {
		h = []store.Change{}
	}
	return h, err
}

type RestoreParams struct {
	ObjectParams
	ChangeID string `json:"change_id" required:"true" desc:"The change to return to, from history.list"`
}

func historyRestore(ctx context.Context, env *command.Env, p RestoreParams) (struct{}, error) {
	return struct{}{}, env.Store.Restore(ctx, "history.restore", p.ObjectType, store.ByID(p.ObjectID), p.ChangeID)
}
