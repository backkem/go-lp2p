package ua

import "fmt"

func NewCLIUserAgent() *mockUserAgent {
	return &mockUserAgent{
		Consumer:  CLICollector,
		Presenter: CLIPresenter,
	}
}

func CLICollector() ([]byte, error) {
	pskEncoded := ""
	fmt.Println("Enter pin:")
	fmt.Scanln(&pskEncoded)

	psk, err := decodeNumeric(pskEncoded)
	if err != nil {
		return nil, err
	}

	return psk, nil
}

func CLIPresenter(psk []byte) {
	pskEncoded := encodeNumeric(psk)
	fmt.Printf("Pin code: %s\n", pskEncoded)
}
