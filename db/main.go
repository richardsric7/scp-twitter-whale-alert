package db

import (
	"log"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// OpenSqliteDB opens ecnrypted SQlite connection
func OpenSqliteDB() (*gorm.DB, error) {

	var errDB error
	// key := "746373408hhgdfdss"
	// if os.Getenv("SQLITE_CYPER_PASSPHRASE") != "" {
	// 	key = os.Getenv("SQLITE_CYPER_PASSPHRASE")
	// }
	dbname := "db/sdb.sqlite"
	// dbnameWithDSN := dbname + fmt.Sprintf("?_pragma_key=%s&_pragma_cipher_page_size=4096", key)

	sqliteDB, errDB := gorm.Open(sqlite.Open(dbname), &gorm.Config{
		Logger:      logger.Default.LogMode(logger.Silent),
		QueryFields: true,
	})

	if errDB != nil {
		log.Printf("[OpenDb]failed to connect database, %s", errDB)
		return nil, errDB
	}

	return sqliteDB, nil
}
