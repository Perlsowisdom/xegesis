package library

import (
	"context"
	"errors"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

type UpdateItemParams struct {
	ID           string    `json:"id" required:"true"`
	Kind         *string   `json:"kind,omitempty" desc:"e.g. book, bible, commentary, monograph, dictionary, lexicon"`
	Title        *string   `json:"title,omitempty"`
	Subtitle     *string   `json:"subtitle,omitempty"`
	Language     *string   `json:"language,omitempty" desc:"BCP 47 tag, e.g. en or grc"`
	Year         *int      `json:"year,omitempty"`
	Publisher    *string   `json:"publisher,omitempty"`
	Series       *string   `json:"series,omitempty"`
	SeriesNo     *string   `json:"series_no,omitempty"`
	ISBN         *string   `json:"isbn,omitempty"`
	Authors      *[]string `json:"authors,omitempty" desc:"Replace the authors with these names, in order"`
	AddFacets    []Facet   `json:"add_facets,omitempty"`
	RemoveFacets []Facet   `json:"remove_facets,omitempty"`
}

func (p UpdateItemParams) fields() (store.Row, error) {
	f := store.Row{}
	text := map[string]*string{"kind": p.Kind, "title": p.Title, "subtitle": p.Subtitle, "language": p.Language,
		"publisher": p.Publisher, "series": p.Series, "series_no": p.SeriesNo, "isbn": p.ISBN}
	for col, v := range text {
		if v != nil {
			f[col] = nullable(strings.TrimSpace(*v))
		}
	}
	if p.Year != nil {
		f["year"] = nullInt(*p.Year)
	}
	for _, col := range []string{"kind", "title"} {
		if v, ok := f[col]; ok && v == nil {
			return nil, command.Errorf("bad_params", "%s cannot be empty", col)
		}
	}
	return f, nil
}

func updateItem(ctx context.Context, env *command.Env, p UpdateItemParams) (Item, error) {
	fields, err := p.fields()
	if err != nil {
		return Item{}, err
	}
	var authors []Person
	cache := map[string]string{}
	if p.Authors != nil {
		var names []string
		for _, n := range *p.Authors {
			names = append(names, n)
			authors = append(authors, Person{Name: n})
		}
		m := Meta{People: authors}
		m.tidy()
		authors = m.People
		if cache, err = peopleByName(ctx, env.Store, names); err != nil {
			return Item{}, err
		}
	}
	current, err := queryStrings(ctx, env.Store, `SELECT person_id FROM item_people WHERE item_id = ? AND role = 'author'`, p.ID)
	if err != nil {
		return Item{}, err
	}
	err = env.Store.Write(ctx, "library.update_item", func(tx *store.Tx) error {
		it, err := tx.Get("items", store.ByID(p.ID))
		if err != nil {
			return err
		}
		if it["deleted_at"] != nil {
			return command.Errorf("not_found", "item %s is deleted", p.ID)
		}
		if p.Authors != nil {
			if err := replaceAuthors(tx, p.ID, current, authors, cache); err != nil {
				return err
			}
		}
		if err := editFacets(tx, p.ID, p.AddFacets, p.RemoveFacets); err != nil {
			return err
		}
		fields["updated_at"] = now()
		return tx.Update("items", store.ByID(p.ID), fields)
	})
	if err != nil {
		return Item{}, err
	}
	return loadItem(ctx, env.Store, p.ID)
}

func replaceAuthors(tx *store.Tx, itemID string, current []string, authors []Person, cache map[string]string) error {
	for _, pid := range current {
		if err := tx.Delete("item_people", store.Row{"item_id": itemID, "person_id": pid, "role": "author"}); err != nil {
			return err
		}
	}
	for i, a := range authors {
		pid, err := personID(tx, cache, a)
		if err != nil {
			return err
		}
		err = tx.Create("item_people", store.Row{"item_id": itemID, "person_id": pid, "role": "author", "position": int64(i)})
		if err != nil {
			return err
		}
	}
	return nil
}

func editFacets(tx *store.Tx, itemID string, add, remove []Facet) error {
	for _, f := range remove {
		err := tx.Delete("item_facets", store.Row{"item_id": itemID, "facet": f.Facet, "value": f.Value})
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return err
		}
	}
	for _, f := range add {
		key := store.Row{"item_id": itemID, "facet": strings.TrimSpace(f.Facet), "value": strings.TrimSpace(f.Value)}
		if key["facet"] == "" || key["value"] == "" {
			return command.Errorf("bad_params", "a facet needs a name and a value")
		}
		if _, err := tx.Get("item_facets", key); err == nil {
			continue
		} else if !errors.Is(err, store.ErrNotFound) {
			return err
		}
		key["origin"] = "user"
		if err := tx.Create("item_facets", key); err != nil {
			return err
		}
	}
	return nil
}
