package argo

import (
	"os"
)

type DomainId uint16

/*
typedef struct xen_argo_addr
{
    xen_argo_port_t aport;	4
    domid_t domain_id;		2
    uint16_t pad;		2
} xen_argo_addr_t;
*/
type Addr struct {
	Port   uint32
	Domain DomainId
	pad    uint16
}

/*
struct argo_ring_id {
        domid_t domain_id;	2
        domid_t partner_id;	2
        xen_argo_port_t aport;	4
};
*/
type RingId struct {
	Domain  DomainId
	Partner DomainId
	Port    uint32
}

/*
typedef struct xen_argo_viptables_rule
{
    struct xen_argo_addr src;	8
    struct xen_argo_addr dst;	8
    uint32_t accept;		4
} xen_argo_viptables_rule_t;
*/
type VIpTablesRule struct {
	Src    Addr
	Dst    Addr
	Accept uint32
}

/*
struct viptables_rule_pos {
    struct xen_argo_viptables_rule* rule;	20
    int position;				4
};
*/
type VIpTablesRulePosition struct {
	Rule     VIpTablesRule
	Position uint32
}

type Conn struct {
	file *os.File
	addr Addr
}

type Listener struct {
	conn *Conn
	ring RingId
}
