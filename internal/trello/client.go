package trello

import (
	"fmt"
	"io"
	"net/http"
)

type Client struct {
	Key    string
	Token  string
	Client *http.Client
}

func NewClient(key, token string) *Client {
	return &Client{
		Key:    key,
		Token:  token,
		Client: &http.Client{},
	}
}

func (c *Client) GetListCards(listID string) ([]byte, error) {
	url := fmt.Sprintf(
		"https://api.trello.com/1/lists/%s/cards?key=%s&token=%s",
		listID,
		c.Key,
		c.Token,
	)

	resp, err := c.Client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}
