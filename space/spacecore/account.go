package spacecore

import (
	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
)

type customAccountService struct {
	account *accountdata.AccountKeys
}

func (c *customAccountService) Init(a *app.App) (err error) {
	return nil
}

func (c *customAccountService) Name() (name string) {
	return accountservice.CName
}

func (c *customAccountService) Account() *accountdata.AccountKeys {
	return c.account
}
