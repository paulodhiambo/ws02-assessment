package main

// Same shape (and field order) as https://jsonplaceholder.typicode.com/users/{id}.

type geo struct {
	Lat string `json:"lat"`
	Lng string `json:"lng"`
}

type address struct {
	Street  string `json:"street"`
	Suite   string `json:"suite"`
	City    string `json:"city"`
	Zipcode string `json:"zipcode"`
	Geo     geo    `json:"geo"`
}

type company struct {
	Name        string `json:"name"`
	CatchPhrase string `json:"catchPhrase"`
	Bs          string `json:"bs"`
}

type user struct {
	ID       int     `json:"id"`
	Name     string  `json:"name"`
	Username string  `json:"username"`
	Email    string  `json:"email"`
	Address  address `json:"address"`
	Phone    string  `json:"phone"`
	Website  string  `json:"website"`
	Company  company `json:"company"`
}

var users = []user{
	{
		ID: 1, Name: "Leanne Graham", Username: "Bret", Email: "Sincere@april.biz",
		Address: address{Street: "Kulas Light", Suite: "Apt. 556", City: "Gwenborough", Zipcode: "92998-3874", Geo: geo{Lat: "-37.3159", Lng: "81.1496"}},
		Phone:   "1-770-736-8031 x56442", Website: "hildegard.org",
		Company: company{Name: "Romaguera-Crona", CatchPhrase: "Multi-layered client-server neural-net", Bs: "harness real-time e-markets"},
	},
	{
		ID: 2, Name: "Ervin Howell", Username: "Antonette", Email: "Shanna@melissa.tv",
		Address: address{Street: "Victor Plains", Suite: "Suite 879", City: "Wisokyburgh", Zipcode: "90566-7771", Geo: geo{Lat: "-43.9509", Lng: "-34.4618"}},
		Phone:   "010-692-6593 x09125", Website: "anastasia.net",
		Company: company{Name: "Deckow-Crist", CatchPhrase: "Proactive didactic contingency", Bs: "synergize scalable supply-chains"},
	},
	{
		ID: 3, Name: "Clementine Bauch", Username: "Samantha", Email: "Nathan@yesenia.net",
		Address: address{Street: "Douglas Extension", Suite: "Suite 847", City: "McKenziehaven", Zipcode: "59590-4157", Geo: geo{Lat: "-68.6102", Lng: "-47.0653"}},
		Phone:   "1-463-123-4447", Website: "ramiro.info",
		Company: company{Name: "Romaguera-Jacobson", CatchPhrase: "Face to face bifurcated interface", Bs: "e-enable strategic applications"},
	},
	{
		ID: 4, Name: "Patricia Lebsack", Username: "Karianne", Email: "Julianne.OConner@kory.org",
		Address: address{Street: "Hoeger Mall", Suite: "Apt. 692", City: "South Elvis", Zipcode: "53919-4257", Geo: geo{Lat: "29.4572", Lng: "-164.2990"}},
		Phone:   "493-170-9623 x156", Website: "kale.biz",
		Company: company{Name: "Robel-Corkery", CatchPhrase: "Multi-tiered zero tolerance productivity", Bs: "transition cutting-edge web services"},
	},
}
