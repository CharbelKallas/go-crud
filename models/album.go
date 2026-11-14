package models

// Album is both the stored record and the JSON shape of the albums API.
// Field names have no json tags on purpose: responses keep the existing
// "ID", "Title", "Artist", "Price" keys, and request bodies accept any
// casing.
//
// Price is stored as DECIMAL(5,2), so it must be between 0 and 999.99.
type Album struct {
	ID     int64
	Title  string  `binding:"required,max=128"`
	Artist string  `binding:"required,max=255"`
	Price  float64 `binding:"gte=0,lte=999.99"`
}
