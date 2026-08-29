package store

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// CreateProduct registers a new product to issue licenses under.
func (s *Store) CreateProduct(name string) (Product, error) {
	p := Product{
		ID:        uuid.NewString(),
		Name:      name,
		CreatedAt: time.Now().UTC(),
	}
	_, err := s.db.Exec(`INSERT INTO products (id, name, created_at) VALUES (?, ?, ?)`,
		p.ID, p.Name, p.CreatedAt)
	if err != nil {
		return Product{}, err
	}
	return p, nil
}

// GetProduct looks up a product by its internal ID.
func (s *Store) GetProduct(id string) (Product, error) {
	var p Product
	err := s.db.QueryRow(`SELECT id, name, created_at FROM products WHERE id = ?`, id).
		Scan(&p.ID, &p.Name, &p.CreatedAt)
	if err == sql.ErrNoRows {
		return Product{}, ErrNotFound
	}
	return p, err
}

// ListProducts returns every product, most recently created first.
func (s *Store) ListProducts() ([]Product, error) {
	rows, err := s.db.Query(`SELECT id, name, created_at FROM products ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var products []Product
	for rows.Next() {
		var p Product
		if err := rows.Scan(&p.ID, &p.Name, &p.CreatedAt); err != nil {
			return nil, err
		}
		products = append(products, p)
	}
	return products, rows.Err()
}
