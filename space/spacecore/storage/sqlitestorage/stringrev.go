package sqlitestorage

// #cgo CFLAGS: -DSQLITE_CORE -I/usr/include/hdf5s/series
// #include "sqlite3.h"
// extern int sqlite3_sqlitereversestring_init(
//        sqlite3 *db,
//        char **pzErrMsg,
//        const sqlite3_api_routines *pApi
//    );
//
// void __attribute__((constructor)) init(void) {
//   sqlite3_auto_extension((void*) sqlite3_sqlitereversestring_init);
// }
import "C"
