package block

import (
	"fmt"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/stretchr/testify/require"
	"strconv"
	"testing"
)


func createBlocks(textArr []string) ([]*model.Block) {
	blocks := []*model.Block{}
	for i := 0; i < len(textArr); i++  {
		blocks = append(blocks, &model.Block{Id: strconv.Itoa(i + 1),
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{ Text: textArr[i] },
			},
		})
	}
	return blocks
}

func createPage(t *testing.T, textArr []string) *pageFixture {
	blocks := createBlocks(textArr)

	fx := newPageFixture(t, blocks...)
	defer fx.ctrl.Finish()
	defer fx.tearDown()

	return fx
}

func checkBlockText(t *testing.T, fx *pageFixture, textArr []string)  {
	//require.Len(t, fx.versions[fx.GetId()].Model().ChildrenIds, len(textArr))
	fmt.Println("HOW IT IS:")
	for i := 0; i < len(fx.versions[fx.GetId()].Model().ChildrenIds); i++  {
		id := fx.versions[fx.GetId()].Model().ChildrenIds[i]
		fmt.Print(fx.versions[id].Model().GetText().Text, " ")
	}
	fmt.Println("\nHOW IT SHOULD BE:")
	for i := 0; i < len(textArr); i++  {
		fmt.Print(textArr[i], " ")
	}

	for i := 0; i < len(textArr); i++  {
		id := fx.versions[fx.GetId()].Model().ChildrenIds[i]
		//fmt.Println("IDs >>> ", fx.versions[fx.GetId()].Model().ChildrenIds)
		require.Equal(t, textArr[i], fx.versions[id].Model().GetText().Text)

		//fmt.Println(fx.versions[id].Model().GetText().Text, "Should be: ", textArr[i])
	}
	fmt.Print("\n")
}

func pasteAny(t *testing.T, fx *pageFixture, id string, textRange model.Range, selectedBlockIds []string, blocks []*model.Block) {
	req := pb.RpcBlockPasteRequest{}
	if id != "" { req.FocusedBlockId = id }
	if len(selectedBlockIds) > 0 { req.SelectedBlockIds = selectedBlockIds }
	req.SelectedTextRange = &textRange
	req.AnySlot = blocks
	err := fx.pasteAny(req)
	require.NoError(t, err)
}

func pasteText(t *testing.T, fx *pageFixture, id string, textRange model.Range, selectedBlockIds []string, textSlot string) {
	req := pb.RpcBlockPasteRequest{}
	if id != "" { req.FocusedBlockId = id }
	if len(selectedBlockIds) > 0 { req.SelectedBlockIds = selectedBlockIds }
	req.TextSlot = textSlot
	req.SelectedTextRange = &textRange
	err := fx.pasteText(req)
	require.NoError(t, err)
}

func checkEvents(t *testing.T, fx *pageFixture, eventsLen int, messagesLen int) {
	//require.Len(t, fx.serviceFx.events, eventsLen)
	//require.Len(t, fx.serviceFx.events[1].Messages, messagesLen)
}

func TestCommonSmart_pasteAny(t *testing.T) {

	t.Run("should split block on paste", func(t *testing.T) {
		fx := createPage(t, []string{"11111", "22222", "33333", "abcde", "55555"})
		pasteAny(t, fx, "4", model.Range{From: 2, To: 4}, []string{}, createBlocks([]string{"22222", "33333"}));

		checkBlockText(t, fx, []string{"11111", "22222", "33333", "ab", "22222", "33333", "e", "55555"});
		checkEvents(t, fx, 2, 5)
	})

	t.Run("should paste to the end when no focus", func(t *testing.T) {
		fx := createPage(t, []string{"11111", "22222", "33333", "44444", "55555"})
		pasteAny(t, fx, "", model.Range{From: 0, To: 0}, []string{}, createBlocks([]string{"22222", "33333"}));

		checkBlockText(t, fx, []string{"11111", "22222", "33333", "44444", "55555", "22222", "33333"});
		checkEvents(t, fx, 2, 3)
	})

	t.Run("should paste to the end when no focus", func(t *testing.T) {
		fx := createPage(t, []string{"11111", "22222", "33333", "44444", "55555"})
		pasteAny(t, fx, "", model.Range{From: 0, To: 0}, []string{"2", "3", "4"}, createBlocks([]string{"22222", "33333"}));

		checkBlockText(t, fx, []string{"11111", "22222", "33333", "55555"});
		checkEvents(t, fx, 2, 6)
	})

	t.Run("should paste to the empty page", func(t *testing.T) {
		fx := createPage(t, []string{})
		pasteAny(t, fx, "", model.Range{From: 0, To: 0}, []string{}, createBlocks([]string{"22222", "33333"}));

		checkBlockText(t, fx, []string{"22222", "33333"});
		checkEvents(t, fx, 2, 6) // TODO
	})

	t.Run("should paste when all blocks selected", func(t *testing.T) {
		fx := createPage(t, []string{"11111", "22222", "33333", "44444", "55555"})
		pasteAny(t, fx, "", model.Range{From: 0, To: 0}, []string{"1", "2", "3", "4", "5"}, createBlocks([]string{"aaaaa", "bbbbb"}));

		checkBlockText(t, fx, []string{"aaaaa", "bbbbb"});
		checkEvents(t, fx, 2, 6)
	})
}

func TestCommonSmart_RangeSplit(t *testing.T) {
	t.Run("1. Курсор в начале, range == 0. Ожидаемое поведение: вставка блоков сверху", func(t *testing.T) {
		fx := createPage(t, []string{ "11111",  "22222",  "33333",  "qwerty",  "55555" })
		pasteAny(t, fx, "4", model.Range{From:0, To:0}, []string{}, createBlocks([]string{ "aaaaa",  "bbbbb" }));

		checkBlockText(t, fx, []string{ "11111", "22222", "33333", "aaaaa", "bbbbb", "qwerty", "55555" });
		checkEvents(t, fx, 2, 6) // TODO
	})

	t.Run("2. Курсор в середине, range == 0. Ожидаемое поведение: разбиение блока на верхний и нижний, вставка посередине", func(t *testing.T) {
		fx := createPage(t, []string{ "11111",  "22222",  "33333",  "qwerty",  "55555" })
		pasteAny(t, fx, "4", model.Range{From:2, To:2}, []string{}, createBlocks([]string{ "aaaaa",  "bbbbb" }));

		checkBlockText(t, fx, []string{ "11111",  "22222",  "33333", "qw",  "aaaaa",  "bbbbb",  "erty", "55555" });
		checkEvents(t, fx, 2, 6) // TODO
	})

	t.Run("3. Курсор в конце, range == 0. Ожидаемое поведение: вставка после блока", func(t *testing.T) {
		fx := createPage(t, []string{ "11111",  "22222",  "33333",  "qwerty",  "55555" })
		pasteAny(t, fx, "4", model.Range{From:6, To:6}, []string{}, createBlocks([]string{ "aaaaa",  "bbbbb" }));

		checkBlockText(t, fx, []string{ "11111", "22222", "33333", "qwerty", "aaaaa", "bbbbb", "55555" });
		checkEvents(t, fx, 2, 6) // TODO
	})

	t.Run("4. Курсор от 1/4 до 3/4, range == 1/2. Ожидаемое поведение: разбиение блока на верхний и нижний, удаление Range, вставка посередине", func(t *testing.T) {
		fx := createPage(t, []string{ "11111",  "22222",  "33333",  "qwerty",  "55555" })
		pasteAny(t, fx, "4", model.Range{From:2, To:4}, []string{}, createBlocks([]string{ "aaaaa",  "bbbbb" }));

		checkBlockText(t, fx, []string{ "11111", "22222", "33333", "qw", "aaaaa", "bbbbb", "ty", "55555" });
		checkEvents(t, fx, 2, 6) // TODO
	})

	t.Run("5. Курсор от начала до середины, range == 1/2. Ожидаемое поведение: вставка сверху, удаление Range", func(t *testing.T) {
		fx := createPage(t, []string{ "11111",  "22222",  "33333",  "qwerty",  "55555" })
		pasteAny(t, fx, "4", model.Range{From:0, To:3}, []string{}, createBlocks([]string{ "aaaaa",  "bbbbb" }));

		checkBlockText(t, fx, []string{ "11111",  "22222",  "33333", "aaaaa", "bbbbb", "rty", "55555" });
		checkEvents(t, fx, 2, 6) // TODO
	})

	t.Run("6. Курсор от середины до конца, range == 1/2. Ожидаемое поведение: вставка снизу, удаление Range", func(t *testing.T) {
		fx := createPage(t, []string{ "11111",  "22222",  "33333",  "qwerty",  "55555" })
		pasteAny(t, fx, "4", model.Range{From:3, To:6}, []string{}, createBlocks([]string{ "aaaaa",  "bbbbb" }));

		checkBlockText(t, fx, []string{ "11111", "22222", "33333", "qwe", "aaaaa", "bbbbb",  "55555" });
		checkEvents(t, fx, 2, 6) // TODO
	})

	t.Run("7. Курсор от начала до конца, range == 1. Ожидаемое поведение: вставка снизу/сверху, удаление блока", func(t *testing.T) {
		fx := createPage(t, []string{ "11111",  "22222",  "33333",  "qwerty",  "55555" })
		pasteAny(t, fx, "4", model.Range{From:0, To:6}, []string{}, createBlocks([]string{ "aaaaa",  "bbbbb" }));

		checkBlockText(t, fx, []string{ "11111",  "22222",  "33333", "aaaaa",  "bbbbb",  "55555" });
		checkEvents(t, fx, 2, 6) // TODO
	})
}

	/*
	=== ANYSLOT PASTE ===
	>>> Тесты на RangeSplit
	БЛОК В ФОКУСЕ, selected text, есть markup

	ПУСТОЙ ДОКУМЕНТ
	8. Возможен ли сценарий, что у нас нет блоков? Возможен, так как title, icon – это виртуальные блоки, их нет на миддле.
	   Нужен тест: отсутствие блоков, нажали CMD+V, ничего не выделено, нет фокуса, нет блоков.

	ВСЕ БЛОКИ ВЫДЕЛЕНЫ
	9. Выделили все блоки, нажали CMD+V

	=== TEXTSLOT PASTE===
	Абзац – текст, не содержащий переносов (\n) строки. Вставляется как фрагмент блока
	Группа абзацев – текст, содержащий как минимум один перенос строки. Вставляется как группа блоков
	10-16. Вставка абзаца для каждого кейса RangeSplit
	17-23. Вставка группы абзацев для каждого кейса RangeSplit
	24. Вставка абзаца, выделена группа блоков
	25. Вставка группы абзацев, выделена группа блоков

	=== TEXTSLOT COPY ===
	26. Выделили фрагмент текстового блока, скорпировали
	27. Выделили целый текстовый блок, скорпировали
	28. Выделили группу текстовых блоков, скопировали
	29. Выделили группу блоков, среди которых есть нетекстовые, скопировали
	30. Выделили группу блоков, среди которых нет текстовых, не скопировали
	 */

func TestCommonSmart_pasteText(t *testing.T) {
	t.Run("should split block on paste", func(t *testing.T) {
		/*fx := createPage(t, []string{"11111", "22222", "33333", "abcde", "55555"})
		pasteAny(t, fx, "4", model.Range{From: 2, To: 4}, []string{}, createBlocks([]string{"22222", "33333"}));

		checkBlockText(t, fx, []string{"11111", "22222", "33333", "ab", "22222", "33333", "e", "55555"});
		checkEvents(t, fx, 2, 5)*/
	})
}