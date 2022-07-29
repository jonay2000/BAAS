// Copyright (c) 2020-2022 TU Delft & Valentijn van de Beek <v.d.vandebeek@student.tudelft.nl> All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sqlite

import (
	errors2 "errors"

	"github.com/baas-project/baas/pkg/util"

	"github.com/baas-project/baas/pkg/model"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

// GetMachineByMac gets any machine with the associated MAC addresses from the database
func (s Store) GetMachineByMac(mac util.MacAddress) (*model.MachineModel, error) {
	machine := model.MachineModel{}
	res := s.Table("machine_models").
		Where("address = ?", mac.Address).
		First(&machine)

	return &machine, res.Error
}

// GetMachines returns the values in the machine_models database.
// TODO: Fetch foreign relations.
func (s Store) GetMachines() (machines []model.MachineModel, _ error) {
	res := s.Find(&machines)
	return machines, res.Error
}

// UpdateMachine updates the information about the machine or creates a machine where one does not yet exist.
func (s Store) UpdateMachine(machine *model.MachineModel) error {
	m, err := s.GetMachineByMac(machine.MacAddress)

	if errors2.Is(err, gorm.ErrRecordNotFound) {
		return s.Save(machine).Error
	} else if err != nil {
		return errors.Wrap(err, "get machine")
	}

	m.Architecture = machine.Architecture
	m.Managed = machine.Managed
	m.Name = machine.Name

	s.Save(&m)
	return nil
}

// CreateMachine creates the machine in the database
func (s Store) CreateMachine(machine *model.MachineModel) error {
	return s.Create(machine).Error
}

func (s Store) DeleteMachine(machine *model.MachineModel) error {
	res := s.Unscoped().Delete(machine)
	return res.Error
}