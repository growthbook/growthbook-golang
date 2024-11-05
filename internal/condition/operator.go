package condition

type Operator string

const (
	andOp Operator = "$and"
	orOp  Operator = "$or"
	norOp Operator = "$nor"
	notOp Operator = "$not"

	eqOp  Operator = "$eq"
	neOp  Operator = "$ne"
	ltOp  Operator = "$lt"
	lteOp Operator = "$lte"
	gtOp  Operator = "$gt"
	gteOp Operator = "$gte"

	veqOp  Operator = "$veq"
	vneOp  Operator = "$vne"
	vgtOp  Operator = "$vgt"
	vgteOp Operator = "$vgte"
	vltOp  Operator = "$vlt"
	vlteOp Operator = "$vlte"

	inOp         Operator = "$in"
	inGroupOp    Operator = "$inGroup"
	ninOp        Operator = "$nin"
	notInGroupOp Operator = "$notInGroup"

	regexOp     Operator = "$regex"
	sizeOp      Operator = "$size"
	elemMatchOp Operator = "$elemMatch"
	allOp       Operator = "$all"
	typeOp      Operator = "$type"
	existsOp    Operator = "$exists"
)
