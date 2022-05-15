package flowfile

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func loadTestingdata(data string) ([]*Block, error) {
	rd := strings.NewReader(data)
	bs, err := ParseBlocks(rd)
	if err != nil {
		return nil, err
	}
	var blocks []*Block
	bs.Foreach(func(b *Block) {
		blocks = append(blocks, b)
	})
	return blocks, nil
}

// Only load part
func TestParseBlocksOnlyLoad(t *testing.T) {
	const testingdata string = `
load file:///root/action1
  load http://localhost:8080/action2

load https://github.com/path/action3

	load 	action4
	`
	blocks, err := loadTestingdata(testingdata)
	if err != nil {
		assert.FailNow(t, err.Error())
	}
	check := func(b *Block, path string) {
		assert.Equal(t, _block_load, b.kind)
		assert.Len(t, b.tokens, 1)
		line := b.tokens[1]
		assert.Len(t, line, 2)

		assert.Equal(t, "load", line[0].word)
		assert.Equal(t, path, line[1].word)

		assert.True(t, line[0].keyword)
		assert.False(t, line[1].keyword)
	}
	check(blocks[0], "file:///root/action1")
	check(blocks[1], "http://localhost:8080/action2")
	check(blocks[2], "https://github.com/path/action3")
	check(blocks[3], "action4")
}

func TestParseBlocksOnlySet(t *testing.T) {
	const testingdata string = `
	set @action1
	input k1=v1 k2=v2
	input k3=v3
	input k=$v

	loop 5 2
end

set @action2 
	input k=$v
	
	input action1_out=$out1
end
	`
	blocks, err := loadTestingdata(testingdata)
	if err != nil {
		assert.FailNow(t, err.Error())
	}
	_ = blocks
}
