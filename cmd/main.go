package main

import (
	"fmt"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/requilence/go-anytype/mobile"
	"github.com/requilence/go-anytype/pb"
)

func main() {

	mobile.SetRepoPath("/Users/roman/.anytypetest")

	mnemonic,err  := mobile.GenerateMnemonic(12)
	if err!=nil{
		panic(err)
	}

	data, err := mobile.WalletAccountAt(mnemonic, 0, "")
	if err!=nil{
		panic(err)
	}

	var w pb.WalletAccountAtResponse
	err = proto.Unmarshal(data, &w)
	if err!=nil{
		panic(err)
	}

	err = mobile.InitRepo(w.Seed)
	if err!=nil{
		panic(err)
	}

	err = mobile.StartAccount(w.Address)
	if err!=nil{
		panic(err)
	}

	time.Sleep(time.Second*5)

	docAdd := pb.AddDocumentConfig{
		Name:                 "Doc1",
		Icon:                 "smile",
		Type:                 "open",
		Sharing:              "shared",
	}
	b, err := proto.Marshal(&docAdd)
	if err!=nil{
		panic(err)
	}


	b, err = mobile.AddDocument(b)
	if err!=nil{
		panic(err)
	}

	var doc pb.Document
	err = proto.Unmarshal(b, &doc)
	if err!=nil{
		panic(err)
	}


	ver := pb.DocumentVersion{
		Name:                 "ver1",
		Icon:                 "smile",
		Blocks:               []*pb.DocumentBlock{
			{
				Id:                   "id1",
				Type:                 pb.DocumentBlockType_EDITABLE,
				ContentType:          pb.DocumentBlockContentType_H1,
				Children:             nil,
				Content:              "Text",
				Width:                "",
			},
		},
	}

	b, err = proto.Marshal(&ver)
	if err!=nil{
		panic(err)
	}

	_, err = mobile.DocumentAddVersion(doc.Id, b)
	if err!=nil{
		panic(err)
	}

	b, err = mobile.DocumentLastVersion(doc.Id)
	if err!=nil{
		panic(err)
	}

	var lastVer pb.DocumentVersion
	err = proto.Unmarshal(b, &lastVer)
	if err!=nil{
		panic(err)
	}

	fmt.Printf("last %s", lastVer.Id)
}
