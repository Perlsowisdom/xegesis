# Book types and traditions

Every book can have a **type** and a **tradition**. Both are item facets (`item_facets`, facets `type` and
`tradition`) with one value each. Nothing asks the user to decide: Xegesis sets them itself, shows why, and
the user changes them when the guess is wrong.

## Where a value came from

| Origin | Meaning |
| --- | --- |
| `metadata` | Set automatically from the book's details, folder or a tradition rule. Has a plain-words reason and a confidence (0 to 1). |
| `bisac` | Set automatically from a subject code found in the file. |
| `user` | Chosen by the user. Always wins: a scan, `facet.suggest` and `facet.rules` never change it. |

Choosing "back to automatic" (`facet.set` with an empty value) removes the user's choice and sets the automatic
value again.

## Types

Bible, Commentary (Old Testament), Commentary (New Testament), Commentary (whole Bible), Dictionary or
encyclopedia, Lexicon or grammar, Theology, Church history, Devotional, Sermons, Biography, Monograph (other),
Fiction, Other.

Monograph (other) and Other are never guessed; they are for the user. A book nothing points to is left without a
type (the book list can filter for "Not set yet").

### What the classifier reads

Clues, strongest first: subjects (including Library of Congress style, "Bible. Romans -- Commentaries"),
series name, title, subtitle, folder names. The strongest clue wins. A commentary is narrowed to a testament by
finding a Bible book in the title or series (`internal/verse` book lookup), looking after the word
"commentary" first so an author's name is not taken for a book. Reasons read like "title mentions a commentary
on Romans".

### Subject codes

The publishing trade's subject-code list needs a paid licence, so it is not embedded. Only three major
headings are mapped, because their meaning is certain: FIC (fiction), BIO (biography), BIB (Bibles). Finer
codes are not guessed.

## Traditions

An editable list (`facet.traditions`), starting with Anglican, Baptist, Catholic, Eastern Orthodox, Evangelical,
Jewish, Lutheran, Methodist, Pentecostal, Presbyterian, Reformed. A tradition used in a choice or a rule joins
the list.

Rules (`facet.rules`): books whose author, publisher or series contains some words, or that sit in a folder
whose name contains them, belong to a tradition. The earliest rule that matches wins. Rules apply to new books
on every scan and to all books when a rule is added, removed or applied; removing a rule takes its automatic
tags back.

## Commands

| Command | Does |
| --- | --- |
| `facet.vocabulary` | The types and the traditions list. |
| `facet.traditions` | List, add or remove a tradition. |
| `facet.suggest` | Set types and rule traditions automatically (all books, a folder or chosen books); `redo` also refreshes earlier automatic values; `preview` only shows. |
| `facet.set` | Set type, tradition or both for many books; empty text goes back to automatic. |
| `facet.rules` | `list`, `add`, `remove`, `apply`. |
| `library.items` | Filters `type`, `tradition` (either may be `none`) and `set_by` (`auto` or `user`). |
