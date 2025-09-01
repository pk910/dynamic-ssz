package types

// Example types demonstrating various SSZ features
type User struct {
	ID       uint64
	Name     []byte    `ssz-max:"64"`           // Dynamic string up to 64 bytes
	Email    []byte    `ssz-max:"256"`          // Email up to 256 bytes  
	PubKey   [48]byte                           // Fixed-size public key
	Balance  [32]byte  `ssz-type:"uint256"`     // 256-bit integer
	Active   bool                               // Boolean flag
	Roles    []byte    `ssz-type:"bitlist" ssz-max:"32"` // Bitlist for roles
}

type Transaction struct {
	From      [20]byte                          // Ethereum address
	To        [20]byte                          // Ethereum address  
	Value     [32]byte  `ssz-type:"uint256"`    // Transfer amount
	Data      []byte    `ssz-max:"1000000"`     // Transaction data
	Nonce     uint64                            // Transaction nonce
	GasLimit  uint64                            // Gas limit
	GasPrice  [32]byte  `ssz-type:"uint256"`    // Gas price
	Signature [65]byte                          // ECDSA signature
}

type Block struct {
	Number       uint64
	Timestamp    uint64  
	ParentHash   [32]byte                       // Previous block hash
	StateRoot    [32]byte                       // State merkle root
	TxRoot       [32]byte                       // Transaction merkle root
	Difficulty   [32]byte  `ssz-type:"uint256"` // Mining difficulty
	Transactions []Transaction `ssz-max:"10000"`  // Block transactions
	Miner        [20]byte                       // Block miner address
	ExtraData    []byte    `ssz-max:"256"`      // Extra block data
}

// Multi-dimensional example
type GameState struct {
	Round      uint32
	IsActive   bool
	
	// 8x8 chess board (fixed 2D array)
	Board      [8][8]uint8
	
	// Dynamic player list
	Players    []Player `ssz-max:"100"`
	
	// Move history (dynamic)
	Moves      []Move   `ssz-max:"1000"`
	
	// Dynamic 2D grid (outer dynamic, inner fixed)
	MapTiles   [][]Tile `ssz-size:"?,16" ssz-max:"256"`
	
	// Bitfield for active players
	ActiveMask []byte   `ssz-type:"bitlist" ssz-max:"100"`
}

type Player struct {
	ID     [16]byte  // Player UUID
	Name   []byte    `ssz-max:"32"`
	Score  uint64
	Team   uint8
}

type Move struct {
	Player    uint8
	FromX     uint8
	FromY     uint8  
	ToX       uint8
	ToY       uint8
	Timestamp uint64
}

type Tile struct {
	Type    uint8
	Height  uint8
	Owner   uint8
	HasFlag bool
}