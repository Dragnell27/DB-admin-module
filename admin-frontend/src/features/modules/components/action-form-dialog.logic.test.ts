// Prueba de la LÓGICA PURA de `ActionFormDialog` en entorno node (sin montar el
// componente). Montar `ActionFormDialog` bajo jsdom/happy-dom cuelga el worker
// fork de Vitest 5.0.3 en Windows (ver `.agents/tasks/fix-hanging-test/spike.json`
// y `.agents/tasks/fix-hanging-test/spike.md`), por lo que la semántica sin DOM
// que el test de render verificaba indirectamente (value elegido, preselección
// por defaultValue, obligatoriedad, solo-editables en modo columnas, string
// verbatim en select/dynamic y coerción numérica) se cubre aquí sobre las
// funciones puras que el componente consume. Mismo patrón que
// `derive-save-error-message.test.ts`.

import { describe, expect, it } from "vitest";

import {
  buildInitialValues,
  buildSubmitInput,
  coerce,
  getVisibleFields,
  isCatalogSelectorField,
  validateRequired,
} from "@/features/modules/components/action-form-dialog.logic";
import type { ColumnField } from "@/features/modules/components/action-form-dialog.logic";
import type { FieldSpec } from "@/features/modules/types";

/** Captura de catálogo (name con prefijo reservado __capture_, INV-6). */
const CAPTURE_FIELD: FieldSpec = {
  name: "__capture_comercio",
  type: "number",
  required: true,
  source: "fillStrategy",
  strategy: {
    kind: "capture-from-catalog",
    catalog: { table: "gt_objetos", idColumn: "obje_numero", labelColumn: "nombre" },
    captureKey: "comercio",
  },
  options: [
    { label: "Tienda A", value: "10" },
    { label: "Tienda B", value: "20" },
  ],
};

/** Derivación (derive-via-fk): el motor la resuelve, el usuario final no la ve. */
const DERIVE_FIELD: FieldSpec = {
  name: "padr_numero",
  type: "number",
  required: false,
  source: "fillStrategy",
  strategy: {
    kind: "derive-via-fk",
    jump: { relatedTable: "gt_valores_padre", localCaptureKey: "comercio", localAttribute: "come_numero", relatedColumn: "valor_objeto" },
    mapping: [{ attribute: "padr_numero", targetColumn: "padr_numero" }],
  },
};

const COUNTRY_FIELD: FieldSpec = {
  name: "country",
  type: "string",
  required: true,
  source: "select",
  options: [
    { label: "Guatemala", value: "gt" },
    { label: "El Salvador", value: "sv" },
  ],
};

const PLAN_FIELD: FieldSpec = {
  name: "plan_id",
  type: "number",
  required: true,
  source: "dynamic",
  dynamicSource: { table: "planes", idColumn: "id", labelColumn: "nombre" },
  options: [
    { label: "Básico", value: "1" },
    { label: "Premium", value: "2" },
  ],
};

/** Monta el estado inicial + submit de la rama inputFields como lo hace el componente. */
function submitInputFields(
  inputFields: FieldSpec[],
  initialValues: Record<string, unknown> = {},
  overrides: Record<string, string | boolean> = {},
): Record<string, unknown> {
  const visibleFields = getVisibleFields(inputFields);
  const values = {
    ...buildInitialValues({
      columnMode: false,
      visibleFields,
      initialValues,
    }),
    ...overrides,
  };
  return buildSubmitInput({ columnMode: false, visibleFields, values });
}

/** Monta el estado inicial + submit de la rama columnMode como lo hace el componente. */
function submitColumnFields(
  columnFields: ColumnField[],
  initialValues: Record<string, unknown> = {},
  overrides: Record<string, string | boolean> = {},
): Record<string, unknown> {
  const values = {
    ...buildInitialValues({
      columnMode: true,
      columnFields,
      visibleFields: [],
      initialValues,
    }),
    ...overrides,
  };
  return buildSubmitInput({ columnMode: true, columnFields, visibleFields: [], values });
}

describe("ActionFormDialog.logic — coerce", () => {
  it("select/dynamic envían el value VERBATIM (string), sin coaccionar por type", () => {
    expect(coerce(COUNTRY_FIELD, "sv")).toBe("sv");
    // dynamic con type:"number" NO coacciona el id a número: viaja como string.
    expect(coerce(PLAN_FIELD, "2")).toBe("2");
  });

  it("boolean → Boolean", () => {
    const f: FieldSpec = { name: "activo", type: "boolean", required: false, source: "input" };
    expect(coerce(f, true)).toBe(true);
    expect(coerce(f, false)).toBe(false);
  });

  it("number → Number, vacío → undefined, NaN passthrough del string crudo", () => {
    const f: FieldSpec = { name: "monto", type: "number", required: false, source: "input" };
    expect(coerce(f, "250")).toBe(250);
    expect(coerce(f, "")).toBeUndefined();
    expect(coerce(f, "x")).toBe("x");
  });

  it("string → raw tal cual", () => {
    const f: FieldSpec = { name: "nombre", type: "string", required: false, source: "input" };
    expect(coerce(f, "hola")).toBe("hola");
  });
});

describe("ActionFormDialog.logic — rama inputFields (select/dynamic)", () => {
  it("envía el value de la opción elegida, no la etiqueta", () => {
    expect(submitInputFields([COUNTRY_FIELD], {}, { country: "sv" })).toEqual({
      country: "sv",
    });
  });

  it("arranca preseleccionado en la opción de defaultValue", () => {
    const init = buildInitialValues({
      columnMode: false,
      visibleFields: getVisibleFields([{ ...COUNTRY_FIELD, defaultValue: "sv" }]),
      initialValues: {},
    });
    expect(init.country).toBe("sv");
  });

  it("dynamic: envía el id (value) VERBATIM como string, nunca la etiqueta", () => {
    expect(submitInputFields([PLAN_FIELD], {}, { plan_id: "2" })).toEqual({
      plan_id: "2",
    });
  });

  it("dynamic: id COLGANTE preseleccionado como string desde initialValues", () => {
    const init = buildInitialValues({
      columnMode: false,
      visibleFields: getVisibleFields([
        {
          ...PLAN_FIELD,
          options: [
            { label: "99", value: "99" },
            { label: "Básico", value: "1" },
            { label: "Premium", value: "2" },
          ],
        },
      ]),
      initialValues: { plan_id: 99 },
    });
    expect(init.plan_id).toBe("99");
  });

  it("el campo destructive envía el value preseleccionado al confirmar", () => {
    const init = buildInitialValues({
      columnMode: false,
      visibleFields: getVisibleFields([{ ...COUNTRY_FIELD, defaultValue: "gt" }]),
      initialValues: {},
    });
    expect(init.country).toBe("gt");
    expect(
      submitInputFields([{ ...COUNTRY_FIELD, defaultValue: "gt" }]),
    ).toEqual({ country: "gt" });
  });

  it("excluye campos constantes de visibles, estado inicial y submit", () => {
    const CONST_FIELD: FieldSpec = {
      name: "country_header",
      type: "string",
      required: true,
      source: "constant",
      constValue: "gt",
    };
    const fields = [COUNTRY_FIELD, CONST_FIELD];
    expect(getVisibleFields(fields).map((f) => f.name)).toEqual(["country"]);
    expect(submitInputFields(fields, {}, { country: "sv" })).toEqual({ country: "sv" });
  });

  it("obligatoriedad: un select requerido vacío produce error", () => {
    const errors = validateRequired({
      columnMode: false,
      visibleFields: getVisibleFields([COUNTRY_FIELD]),
      values: { country: "" },
    });
    expect(errors).toEqual({ country: "Este campo es obligatorio" });
  });

  it("obligatoriedad: boolean nunca cuenta como vacío", () => {
    const f: FieldSpec = { name: "activo", type: "boolean", required: true, source: "input" };
    const errors = validateRequired({
      columnMode: false,
      visibleFields: getVisibleFields([f]),
      values: { activo: false },
    });
    expect(errors).toEqual({});
  });
});

describe("ActionFormDialog.logic — rama columnMode", () => {
  const ID_COL: ColumnField = {
    field: "id",
    label: "ID",
    type: "string",
    readOnly: true,
    required: true,
  };
  const MONTO_COL: ColumnField = {
    field: "monto",
    label: "Monto",
    type: "number",
    readOnly: false,
    required: false,
  };

  it("precarga la columna readOnly (PK) desde la fila y la editable", () => {
    const init = buildInitialValues({
      columnMode: true,
      columnFields: [ID_COL, MONTO_COL],
      visibleFields: [],
      initialValues: { id: 42, monto: 100 },
    });
    expect(init.id).toBe("42");
    expect(init.monto).toBe("100");
  });

  it("solo viajan las columnas editables; la PK readOnly se excluye y se coacciona el número", () => {
    expect(
      submitColumnFields([ID_COL, MONTO_COL], { id: 42, monto: 100 }, { monto: "250" }),
    ).toEqual({ monto: 250 });
  });

  it("columna con options: preselecciona por defaultValue y envía el value string", () => {
    const ESTADO_COL: ColumnField = {
      field: "estado",
      label: "Estado",
      type: "string",
      readOnly: false,
      required: true,
      options: [
        { label: "Activo", value: "1" },
        { label: "Inactivo", value: "2" },
      ],
      defaultValue: "1",
    };
    const init = buildInitialValues({
      columnMode: true,
      columnFields: [ID_COL, ESTADO_COL],
      visibleFields: [],
      initialValues: { id: 7 },
    });
    expect(init.estado).toBe("1");
    expect(
      submitColumnFields([ID_COL, ESTADO_COL], { id: 7 }, { estado: "2" }),
    ).toEqual({ estado: "2" });
  });

  it("columna con options numérica: el value NO se coacciona (viaja string)", () => {
    const LISTA_NUM: ColumnField = {
      field: "plan",
      label: "Plan",
      type: "number",
      readOnly: false,
      required: true,
      options: [{ label: "Premium", value: "2" }],
    };
    expect(submitColumnFields([LISTA_NUM], {}, { plan: "2" })).toEqual({ plan: "2" });
  });

  it("obligatoriedad: solo valida columnas editables (la readOnly requerida no falla)", () => {
    const errors = validateRequired({
      columnMode: true,
      columnFields: [ID_COL, { ...MONTO_COL, required: true }],
      visibleFields: [],
      values: { id: "", monto: "" },
    });
    expect(errors).toEqual({ monto: "Este campo es obligatorio" });
  });
});

describe("ActionFormDialog.logic — fillStrategy (capturas de catálogo, INV-6)", () => {
  it("isCatalogSelectorField: select/dynamic y capture-from-catalog son selectores; derive no", () => {
    expect(isCatalogSelectorField(COUNTRY_FIELD)).toBe(true);
    expect(isCatalogSelectorField(PLAN_FIELD)).toBe(true);
    expect(isCatalogSelectorField(CAPTURE_FIELD)).toBe(true);
    expect(isCatalogSelectorField(DERIVE_FIELD)).toBe(false);
  });

  it("getVisibleFields muestra la captura (incl. __capture_*) y oculta las derivaciones", () => {
    const CONST_FILL: FieldSpec = {
      name: "estado",
      type: "string",
      required: false,
      source: "fillStrategy",
      strategy: { kind: "constant" },
    };
    const AUTO_FILL: FieldSpec = {
      name: "hijo_numero",
      type: "number",
      required: false,
      source: "fillStrategy",
      strategy: { kind: "autogenerated" },
    };
    const visible = getVisibleFields([CAPTURE_FIELD, DERIVE_FIELD, CONST_FILL, AUTO_FILL]);
    expect(visible.map((f) => f.name)).toEqual(["__capture_comercio"]);
  });

  it("coerce: la captura envía el VALUE elegido (id) verbatim como string, sin coaccionar a número", () => {
    expect(coerce(CAPTURE_FIELD, "20")).toBe("20");
  });

  it("buildInitialValues: preselecciona el id colgante de la captura como string", () => {
    const init = buildInitialValues({
      columnMode: false,
      visibleFields: getVisibleFields([CAPTURE_FIELD]),
      initialValues: { __capture_comercio: 20 },
    });
    expect(init.__capture_comercio).toBe("20");
  });

  it("buildSubmitInput: solo viaja la captura (input[name]=id); las derivaciones no", () => {
    const fields = [CAPTURE_FIELD, DERIVE_FIELD];
    const visibleFields = getVisibleFields(fields);
    const values = {
      ...buildInitialValues({ columnMode: false, visibleFields, initialValues: {} }),
      __capture_comercio: "10",
    };
    expect(buildSubmitInput({ columnMode: false, visibleFields, values })).toEqual({
      __capture_comercio: "10",
    });
  });

  it("obligatoriedad: una captura requerida vacía produce error", () => {
    const errors = validateRequired({
      columnMode: false,
      visibleFields: getVisibleFields([CAPTURE_FIELD]),
      values: { __capture_comercio: "" },
    });
    expect(errors).toEqual({ __capture_comercio: "Este campo es obligatorio" });
  });
});
