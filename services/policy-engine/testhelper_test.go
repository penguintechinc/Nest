package main

import (
	"github.com/glebarez/sqlite"
	"github.com/penguintechinc/nest/shared/database"
	"gorm.io/gorm"
)

// getTestDAL returns an in-memory SQLite-backed PenguinDAL for tests.
func getTestDAL() *database.PenguinDAL {
	db, _ := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	return database.NewPenguinDAL(db)
}
