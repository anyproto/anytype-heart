### Run example

**Preconditions**

1. Install ts-node: `sudo npm i -g ts-node` to run `.ts` files in one step.
2. Install js packages: `npm i`.
3. Generate static ts/go modules from the `.protocol` files.

**Then, simply run**

1. `npm run start@go`
2. `npm run start@ts`

### TS <- Proto generation

```
pbjs -t static-module -w commonjs -o build/ts/event.js event.proto
pbts -o build/ts/event.d.ts build/ts/event.js
```

Additionally, TypeScript definitions of static modules are compatible with their reflection-based counterparts (i.e. as exported by JSON modules), as long as the following conditions are met:

1. Instead of using `new SomeMessage(...)`, always use `SomeMessage.create(...)` because reflection objects do not provide a constructor.
2. Types, services and enums must start with an uppercase letter to become available as properties of the reflected types as well (i.e. to be able to use `MyMessage.MyEnum` instead of `root.lookup("MyMessage.MyEnum"))`.

### GO <- Proto generation

```
protoc -I protocol/ protocol/event.proto --go_out=plugins=grpc:protocol/build/go
```

### User stories

#### 1. Холодный запуск
#### 2. Горячий запуск
#### 3A. Получение списка документов (если store контролирует клиент)
Нужно получить список id документов, их имена, аватарки, хеши последних актуальных версий 
Когда нужен этот сценарий? Когда юзер хочет запустить главный экран.

1. Юзер запустил приложение. Middle уже авторизован, пока ничего не отрисовано
2. Фронт сообщает о том, какие у него документы есть 

```js
    Front: Message StartUp (docs: [
        {root:0x345, last_ver:0x123}, 
        {root:0x456, last_ver:0x234}, 
    ...])
```

3. Миддл сообщает, какие документы поменяли имена/аватарки, присылает их, актуальная ли версия хранимого документа, и если нет, то какая актуальная (или массив хешей CRDT-изменений, которые нужно скачать для восстановления до актуальной версии)

```js
    Middle: Message StartUp reply (docs: [
        {root:0x345, status:last_version}, 
        {root:0x456, status:outdated, name:same, icon:b64(newIcon.png), lastVersion:0x789},
    ...])
```

4. Клиент применяет полученные изменения и отображает список документов

#### 3B. Получение списка документов (если store контролирует middle)
Не вижу проблемы, если middle будет контролировать store. Плюсы – логика с клиента переходит на middle.

1. Юзер запустил приложение. Middle уже авторизован, пока ничего не отрисовано
2. Клиент сообщает, что он запустился

```js
    Front: Message StartUp ()
```

3. Middle отдает данные, которые нужно отрисовать на главной странице – список документов

```js
    Middle: Message DocumentsOrganizier (docs: [
        {name:'Doc 1', version:0x123, icon:icon1.png},
        {name:'Doc 2', version:0x234, icon:icon2.png},
    ...])
```

Логика по получению актуальных версий, сверки и прочего полностью абстрагирована от клиента.

4. Клиент просто отрисовывает полученные данные.

##### Cообщения сценария
1. Сообщение, которым клиент сообщает, что ему нужен отрисовать список документов. Возникает в сценариях, когда мы на главном меню, плюс, возможно, в других сценариях (например, какое-то всплывающее контекстное меню, в котором отображаются документы).
2. Сообщение, в котором middle передает список всех документов.

```js
    // С помощью запроса с entity == docHeaders можно запросить список документов
    // Выделять отдельное в сообщение DocumentsRequest не вижу смысла, оно слишком тривиальное получится
    message Request {
        string entity = 1;
    }

    message DocHeaders {
        string id = 0;
        repeated DocHeader docHeaders = 1;
        string target = 2;
    }

    message DocHeader {
        string id = 0;
        string name = 1;
        string root = 2;
        string version = 3;
        string iconName = 4;
    }
```
