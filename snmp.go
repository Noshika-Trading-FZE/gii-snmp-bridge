package main

import (
	g "github.com/gosnmp/gosnmp"
	log "github.com/sirupsen/logrus"
)

// const enterprize = ".1.3.6.1.4.1"
const (
	TrapOID  = ".1.3.6.1.6.3.1.1.4.1.0"
	LinkUp   = ".1.3.6.1.6.3.1.1.5.4"
	LinkDown = ".1.3.6.1.6.3.1.1.5.3"
	SysName  = ".1.3.6.1.2.1.1.5"
)

type SNMP struct {
	Stream chan g.SnmpTrap
	Host   string
	Port   int
}

func NewSNMP(host string, port int) *SNMP {
	return &SNMP{
		Stream: make(chan g.SnmpTrap, 64),
		Host:   host,
		Port:   port,
	}
}

func (t *SNMP) Run() {
	g.Default.Target = t.Host
	g.Default.Port = uint16(t.Port)
	g.Default.Version = g.Version2c
	g.Default.Community = "public"
	g.Default.Logger = g.NewLogger(log.StandardLogger())
	err := g.Default.Connect()
	if err != nil {
		log.Errorf("Connect() err: %v", err)
	}
	defer g.Default.Conn.Close()

	for {
		trap := <-t.Stream

		_, err = g.Default.SendTrap(trap)
		if err != nil {
			log.Errorf("SendTrap() err: %v", err)
		}
	}
}

func (t *SNMP) TrapRouter(name string, lost bool) g.SnmpTrap {
	pdu := g.SnmpPDU{
		Name:  TrapOID,
		Type:  g.ObjectIdentifier,
		Value: LinkUp,
	}

	if lost {
		pdu.Value = LinkDown
	}

	sysDesc := g.SnmpPDU{
		Name:  SysName,
		Type:  g.OctetString,
		Value: name,
	}

	trap := g.SnmpTrap{
		Variables: []g.SnmpPDU{pdu, sysDesc},
	}

	return trap
}

func (t *SNMP) TrapLNS(lost bool) g.SnmpTrap {
	pdu := g.SnmpPDU{
		Name:  TrapOID,
		Type:  g.ObjectIdentifier,
		Value: LinkUp,
	}

	if lost {
		pdu.Value = LinkDown
	}

	sysDesc := g.SnmpPDU{
		Name:  SysName,
		Type:  g.OctetString,
		Value: "LNS",
	}

	trap := g.SnmpTrap{
		Variables: []g.SnmpPDU{pdu, sysDesc},
	}

	return trap
}
