
typedef void (*voidFunc) (char*, char*, int);

voidFunc clientFunc;

void setClientFunc(voidFunc f)
{
	clientFunc = f;
}

void CallClientFunc(char* operation, char* data, int data_len)
{
    clientFunc(operation, data, data_len);
}
