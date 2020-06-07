package main


import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/gob"
	"flag"
	"fmt"
	"github.com/boltdb/bolt"
	"log"
	"math"
	"math/big"
	"os"
	"strconv"
	"time"
)
const dbFile = "blockchain.db"
const blocksBucket = "blocks"
const targetBits = 24
//创建block与chain
type Block struct {
	Timestamp     int64
	Data          []byte
	PrevBlockHash []byte
	Hash          []byte
	Nonce         int
}
type Blockchain struct {
	tip []byte
	db  *bolt.DB
}
//CLI command
type CLI struct {
	bc *Blockchain
}
//生成block与blockchain
func NewBlock(data string, prevBlockHash []byte) *Block {
	block := &Block{
		Timestamp:     time.Now().Unix(),
		Data:          []byte(data),
		PrevBlockHash: prevBlockHash,
		Hash:          []byte{},
		Nonce:         0}
	pow := NewProofOfWork(block)
	nonce, hash := pow.Run()
	block.Nonce = nonce
	block.Hash = hash[:]
	return block
}

func NewBlockchain() *Blockchain {
	var tip []byte
	// 打开 BoltDB 文件的标准操作
	db, err := bolt.Open(dbFile, 0600, nil)
	if err != nil {
		log.Panic(err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(blocksBucket))//打开bucket

		// 如果数据库中不存在区块链就创建一个，否则直接读取最后一个块的哈希
		if b == nil {
			fmt.Println("No existing blockchain found. Creating a new one...")
			genesis :=NewBlock("First block", []byte{})//生成创世区块

			b, err := tx.CreateBucket([]byte(blocksBucket))//创建block的bucket
			if err != nil {
				log.Panic(err)
			}

			err = b.Put(genesis.Hash, genesis.Serialize())//建立hash与block的键值对
			if err != nil {
				log.Panic(err)
			}

			err = b.Put([]byte("l"), genesis.Hash)//建立l与hash的键值对
			if err != nil {
				log.Panic(err)
			}
			tip = genesis.Hash//将tip指向最后的hash
		} else {
			tip = b.Get([]byte("l"))//如果纯在，直接读取
		}

		return nil
	})

	if err != nil {
		log.Panic(err)
	}

	bc := Blockchain{tip, db}

	return &bc
}

// 将 Block 序列化为一个字节数组
func (b *Block) Serialize() []byte {
	var result bytes.Buffer
	encoder := gob.NewEncoder(&result)
	err := encoder.Encode(b)
	if err != nil {
		log.Panic(err)
	}
	return result.Bytes()
}

// 将字节数组反序列化为一个 Block
func DeserializeBlock(d []byte) *Block {
	var block Block

	decoder := gob.NewDecoder(bytes.NewReader(d))
	err := decoder.Decode(&block)
	if err != nil {
		log.Panic(err)
	}
	return &block
}







// 加入区块
func (bc *Blockchain) AddBlock(data string) {
	var lastHash []byte
	// 首先获取最后一个块的哈希用于生成新块的哈希
	err := bc.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(blocksBucket))
		lastHash = b.Get([]byte("l"))
		return nil
	})

	if err != nil {
		log.Panic(err)
	}

	newBlock := NewBlock(data, lastHash)

	err = bc.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(blocksBucket))
		err := b.Put(newBlock.Hash, newBlock.Serialize())
		if err != nil {
			log.Panic(err)
		}

		err = b.Put([]byte("l"), newBlock.Hash)
		if err != nil {
			log.Panic(err)
		}

		bc.tip = newBlock.Hash

		return nil
	})
}
//为了能顺序读取bolt中的数据，我们创建了迭代器
type BlockchainIterator struct {
	currentHash []byte
	db          *bolt.DB
}

func (bc *Blockchain) Iterator() *BlockchainIterator {
	bci := &BlockchainIterator{bc.tip, bc.db}

	return bci
}

// 返回链中的下一个块
func (i *BlockchainIterator) Next() *Block {
	var block *Block
	err := i.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(blocksBucket))
		encodedBlock := b.Get(i.currentHash)
		block = DeserializeBlock(encodedBlock)
		return nil
	})

	if err != nil {
		log.Panic(err)
	}

	i.currentHash = block.PrevBlockHash

	return block
}



const usage = `
Usage:
  addblock -data BLOCK_DATA    添加区块，命令为addblock -data，之后空格并输入数据
  printchain                   呈现出所有的block的信息（模仿bilibili评论按照时间排序，最新的在最上面）
  print_someblock -num  NUM    呈现出某个block的信息（最古老的是第一个），some有某个的意思····（没想到居然会这么复习英语）
`

func (cli *CLI) printUsage() {
	fmt.Println(usage)
}

func (cli *CLI) validateArgs() {
	if len(os.Args) < 2 {
		cli.printUsage()
		os.Exit(1)
	}
}

func (cli *CLI) Run() {
	cli.validateArgs()

	addBlockCmd := flag.NewFlagSet("addblock", flag.ExitOnError)
	printChainCmd := flag.NewFlagSet("printchain", flag.ExitOnError)
	print_someblockCmd := flag.NewFlagSet("print_someblock", flag.ExitOnError)

	addBlockData := addBlockCmd.String("data", "", "Block data")
	printblock := print_someblockCmd.String("num", "", "NUM")

	switch os.Args[1] {
	case "addblock":
		err := addBlockCmd.Parse(os.Args[2:])
		if err != nil {
			log.Panic(err)
		}
	case "printchain":
		err := printChainCmd.Parse(os.Args[2:])
		if err != nil {
			log.Panic(err)
		}
	case "print_someblock":
		err := print_someblockCmd.Parse(os.Args[2:])
		if err != nil {
			log.Panic(err)
		}
	default:
		cli.printUsage()
		os.Exit(1)
	}

	if addBlockCmd.Parsed() {
		if *addBlockData == "" {
			addBlockCmd.Usage()
			os.Exit(1)
		}
		cli.bc.AddBlock(*addBlockData)
	}

	if printChainCmd.Parsed() {
		cli.printChain()
	}

	if print_someblockCmd.Parsed() {
		if *printblock == "" {
			print_someblockCmd.Usage()
			os.Exit(1)
		}
		cli.print_someblock(*printblock)
	}
}


func (cli *CLI) addBlock(data string) {
	cli.bc.AddBlock(data)
	fmt.Println("Success")
}

func (cli *CLI) printChain() {
	bci := cli.bc.Iterator()

	for {
		block := bci.Next()

		fmt.Printf("Prev hash: %x\n", block.PrevBlockHash)
		fmt.Printf("Data: %s\n", block.Data)
		fmt.Printf("Hash: %x\n", block.Hash)
		pow := NewProofOfWork(block)
		fmt.Printf("PoW: %s\n", strconv.FormatBool(pow.Validate()))
		fmt.Println()

		if len(block.PrevBlockHash) == 0 {
			break
		}
	}
}
func (cli *CLI) print_someblock(num string) {
	number:=0
	length:=0
	Num, err:= strconv.Atoi(num)
	if(err==nil){
		Num=Num
	}
	bci := cli.bc.Iterator()
	counter:= cli.bc.Iterator()

	for {
		block1 := counter.Next()
		length=length+1
		if len(block1.PrevBlockHash) == 0 {
			break
		}
	}

	for {
		block := bci.Next()
		if((length-Num)!=number){
			continue
		}
		fmt.Printf("Prev hash: %x\n", block.PrevBlockHash)
		fmt.Printf("Data: %s\n", block.Data)
		fmt.Printf("Hash: %x\n", block.Hash)
		pow := NewProofOfWork(block)
		fmt.Printf("PoW: %s\n", strconv.FormatBool(pow.Validate()))
		fmt.Println()
		if(Num==1){
			break
		}
		if len(block.PrevBlockHash) == 0 {
			fmt.Println("输入的数字有误，请查看是否有效或者超出区块链总长度")
			break
		}
		break
	}
}



var (
	maxNonce = math.MaxInt64
)
//感觉不放到最前面是不是更有整体的感觉，放最前面感觉写着写着就忘了
type ProofOfWork struct {
	block  *Block
	target *big.Int
}

func NewProofOfWork(b *Block) *ProofOfWork {
	target := big.NewInt(1)
	target.Lsh(target, uint(256-targetBits))

	pow := &ProofOfWork{b, target}

	return pow
}


//pow的入口，不想注释了····
func (pow *ProofOfWork) Run() (int, []byte) {
	var hashInt big.Int
	var hash [32]byte
	nonce := 0
	fmt.Printf("Mining the block containing \"%s\"\n", pow.block.Data)
	for nonce < maxNonce {
		data := bytes.Join(
			[][]byte{
				pow.block.PrevBlockHash,
				pow.block.Data,
				IntToHex(pow.block.Timestamp),
				IntToHex(int64(targetBits)),
				IntToHex(int64(nonce)),
			},
			[]byte{},
		)
		hash = sha256.Sum256(data)
		hashInt.SetBytes(hash[:])

		if hashInt.Cmp(pow.target) == -1 {
			fmt.Printf("\r%x", hash)
			break
		} else {
			nonce++
		}
	}
	fmt.Print("\n\n")

	return nonce, hash[:]
}

// Validate block's PoW
func (pow *ProofOfWork) Validate() bool {
	var hashInt big.Int
	data := bytes.Join(
		[][]byte{
			pow.block.PrevBlockHash,
			pow.block.Data,
			IntToHex(pow.block.Timestamp),
			IntToHex(int64(targetBits)),
			IntToHex(int64(pow.block.Nonce)),
		},
		[]byte{},
	)
	hash := sha256.Sum256(data)
	hashInt.SetBytes(hash[:])

	isValid := hashInt.Cmp(pow.target) == -1

	return isValid
}


func IntToHex(num int64) []byte {
	buff := new(bytes.Buffer)
	err := binary.Write(buff, binary.BigEndian, num)
	if err != nil {
		log.Panic(err)
	}

	return buff.Bytes()
}

func main() {
	bc := NewBlockchain()
	defer bc.db.Close()

	cli := CLI{bc}
	cli.Run()
}
