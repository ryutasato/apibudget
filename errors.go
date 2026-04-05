package apibudget

import "errors"

var (
	// ErrInsufficientCredits はクレジット残高不足を示す。
	// Confirm時に事後超過が発生した場合にも返される（消費自体は反映済み）。
	ErrInsufficientCredits = errors.New("apibudget: insufficient credits")

	// ErrReservationAlreadyFinalized は予約が既にConfirmまたはCancel済みであることを示す。
	ErrReservationAlreadyFinalized = errors.New("apibudget: reservation already finalized")

	// ErrAPINotFound は指定されたAPI名が登録されていないことを示す。
	ErrAPINotFound = errors.New("apibudget: api not found")

	// ErrPoolNotFound は指定されたクレジットプール名が登録されていないことを示す。
	ErrPoolNotFound = errors.New("apibudget: credit pool not found")
)
