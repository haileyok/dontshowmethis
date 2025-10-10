package main

import "gorm.io/gorm"

type LogItem struct {
	gorm.Model
	ParentDid  string `gorm:"index"`
	AuthorDid  string `gorm:"index"`
	ParentUri  string `gorm:"index"`
	AuthorUri  string
	ParentText string
	AuthorText string
	Label      string `gorm:"index"`
}
