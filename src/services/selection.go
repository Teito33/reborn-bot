package services

import (
	"discord-bot/src/models"
	"errors"
)

func SelectPlayers(signups []models.Player) (models.Player, models.Player, []models.Player, error) {
	var tank models.Player
	var healer models.Player
	var dpsPlayers []models.Player

	for _, player := range signups {
		switch player.Role {
		case "TankR":
			if tank.Name == "" {
				tank = player
			}
		case "HealR":
			if healer.Name == "" {
				healer = player
			}
		case "DpsR":
			if len(dpsPlayers) < 2 {
				dpsPlayers = append(dpsPlayers, player)
			}
		}
	}

	if tank.Name == "" {
		return models.Player{}, models.Player{}, nil, errors.New("no tank selected")
	}
	if healer.Name == "" {
		return models.Player{}, models.Player{}, nil, errors.New("no healer selected")
	}
	if len(dpsPlayers) < 2 {
		return models.Player{}, models.Player{}, nil, errors.New("not enough DPS players selected")
	}

	return tank, healer, dpsPlayers, nil
}
