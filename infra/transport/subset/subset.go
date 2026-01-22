package subset

import "github.com/webitel/im-thread-service/infra/transport/consistent"

func Subset[M consistent.Member](selectKey string, inss []M, num int) []M {
	if len(inss) <= num {
		return inss
	}

	c := consistent.New[M]()
	{
		c.NumberOfReplicas = 160
		c.UseFnv = true
		c.Set(inss)
	}

	return subset(c, selectKey, inss, num)
}

func subset[M consistent.Member](c *consistent.Consistent[M], selectKey string, insss []M, num int) []M {
	backends, err := c.GetN(selectKey, num)
	if err != nil {
		return insss
	}

	return backends
}
