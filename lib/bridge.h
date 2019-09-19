
typedef void (*voidFunc) (char*, char*, int);

void setClientFunc(voidFunc f);

void CallClientFunc(char* operation, char* data, int data_len);
