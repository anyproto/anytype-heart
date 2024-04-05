/*
 * https://github.com/mayflower/sqlite-reverse-string
 * The MIT License (MIT)
 *
 * Copyright (c) 2015 Christian Speckner
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */



#include <stdlib.h>
#include <sqlite3ext.h>

SQLITE_EXTENSION_INIT1

#ifndef SQLITE_CORE
#ifdef _WIN32
__declspec(dllexport)
#endif
#endif

// #############################################################################
// UTF-8 stuff below taken from sqlite3 utf.c
// #############################################################################

static const unsigned char sqlite3Utf8Trans1[] = {
  0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
  0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
  0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17,
  0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
  0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
  0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
  0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
  0x00, 0x01, 0x02, 0x03, 0x00, 0x01, 0x00, 0x00,
};

#define READ_UTF8(zIn, zTerm, c)                           \
  c = *(zIn++);                                            \
  if( c>=0xc0 ){                                           \
    c = sqlite3Utf8Trans1[c-0xc0];                         \
    while( zIn!=zTerm && (*zIn & 0xc0)==0x80 ){            \
      c = (c<<6) + (0x3f & *(zIn++));                      \
    }                                                      \
    if( c<0x80                                             \
        || (c&0xFFFFF800)==0xD800                          \
        || (c&0xFFFFFFFE)==0xFFFE ){  c = 0xFFFD; }        \
  }


#define WRITE_UTF8(zOut, c) {                          \
  if( c<0x00080 ){                                     \
    *zOut++ = (unsigned char)(c&0xFF);                 \
  }                                                    \
  else if( c<0x00800 ){                                \
    *zOut++ = 0xC0 + (unsigned char)((c>>6)&0x1F);     \
    *zOut++ = 0x80 + (unsigned char)(c & 0x3F);        \
  }                                                    \
  else if( c<0x10000 ){                                \
    *zOut++ = 0xE0 + (unsigned char)((c>>12)&0x0F);    \
    *zOut++ = 0x80 + (unsigned char)((c>>6) & 0x3F);   \
    *zOut++ = 0x80 + (unsigned char)(c & 0x3F);        \
  }else{                                               \
    *zOut++ = 0xF0 + (unsigned char)((c>>18) & 0x07);  \
    *zOut++ = 0x80 + (unsigned char)((c>>12) & 0x3F);  \
    *zOut++ = 0x80 + (unsigned char)((c>>6) & 0x3F);   \
    *zOut++ = 0x80 + (unsigned char)(c & 0x3F);        \
  }                                                    \
}

// #############################################################################

static sqlite_uint64 strlen_utf8(const unsigned char* str) {
    sqlite_uint64 len = 0;
    int c = 1;

    while (c) {
        READ_UTF8(str, 0, c);

        if (c) len++;
    }

    return len;
}

static void decode_utf8(const unsigned char* str, int* buffer) {
    int c = 1;

    while (c) {
        READ_UTF8(str, 0, c);

        if (c) *(buffer++) = c;
    }
}

static void reverse_string(int* buffer, char* result, sqlite_uint64 len) {
    sqlite3_uint64 i;
    for (i = 0; i < len; i++) {
        WRITE_UTF8(result, buffer[len - i - 1]);
    }
    *result = 0;
}

static void result_string_destructor(void* result_string) {
    sqlite3_free(result_string);
}

static void string_reverse_implementation(sqlite3_context* ctx, int argc, sqlite3_value** argv) {
    const unsigned char* input = 0;
    int input_type;
    sqlite_uint64 input_length;
    char* result;
    int* decoded;

    if (argc < 1) {
        sqlite3_result_error(ctx, "not enough parameters", -1);
        return;
    }

    if (argc > 1) {
        sqlite3_result_error(ctx, "too many parameters", -1);
        return;
    }

    input_type = sqlite3_value_type(argv[0]);

    if (input_type != SQLITE_NULL) {
        input = sqlite3_value_text(argv[0]);
    }

    if (input == 0) {
        sqlite3_result_null(ctx);
        return;
    }

    input_length = strlen_utf8(input);
    result = sqlite3_malloc(4 * input_length + 1);
    decoded = sqlite3_malloc(sizeof(int) * input_length);

    decode_utf8(input, decoded);
    reverse_string(decoded, result, input_length);

    sqlite3_free(decoded);

    sqlite3_result_text(ctx, result, -1, result_string_destructor);
}

int sqlite3_sqlitereversestring_init(
    sqlite3 *db,
    char **pzErrMsg,
    const sqlite3_api_routines *pApi
){
    int rc = SQLITE_OK;
    SQLITE_EXTENSION_INIT2(pApi);

    sqlite3_create_function(db, "reverse_string", 1, SQLITE_UTF8, 0, string_reverse_implementation, 0, 0);

    return rc;
}