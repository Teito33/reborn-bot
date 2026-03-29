package models

type Player struct {
	Name string
	Role string
}

func NewPlayer(name string, role string) *Player {
	return &Player{
		Name: name,
		Role: role,
	}
}
