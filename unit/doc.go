// Package unit provides a "work unit" scheduling system for handling data sets that traverse multiple workers / goroutines.
// The aim is to bind priority to a data set instead of a goroutine and split resources fairly among requests.
package unit
