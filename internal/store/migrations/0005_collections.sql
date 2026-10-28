-- library.db, schema version 5.
-- Collections count books by type, tradition and so on over the whole library; an index that holds
-- the book's ID as well lets those counts be read from the index alone.

CREATE INDEX item_facets_by_value ON item_facets(facet, value, item_id);
CREATE INDEX item_people_by_person ON item_people(person_id, item_id, role);
CREATE INDEX licences_by_type ON licences(type, item_id);
