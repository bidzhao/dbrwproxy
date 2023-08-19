//go:build go1.19
// +build go1.19

package mysql

import "sync/atomic"

/******************************************************************************
*                               Sync utils                                    *
******************************************************************************/

type atomicBool = atomic.Bool
