"use client";

/**
 * Diálogo con formulario autogenerado para ejecutar una acción de un módulo
 * dinámico (module-builder, tarea 14.3, Req. 10.1, 10.3, 10.4).
 *
 * Genera un control por cada campo de entrada de la acción según su tipo
 * (string/number/boolean/date), valida la obligatoriedad antes de enviar y, al
 * confirmar, entrega el input al padre (que dispara `useModuleAction`). Se
 * construye con primitivas existentes (`Card`, `Button`, `Input`, `Label`),
 * siguiendo el patrón del `confirm-delete-dialog` del proyecto (modal
 * controlado, cierre con Escape/telón).
 */
import * as React from "react";
import { AlertCircle, AlertTriangle, Loader2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import type { ModuleAction } from "@/features/modules/types";
import {
  buildInitialValues,
  buildSubmitInput,
  getVisibleFields,
  validateRequired,
} from "@/features/modules/components/action-form-dialog.logic";
import type { ColumnField } from "@/features/modules/components/action-form-dialog.logic";

// Reexporta el tipo (fuente única en el módulo de lógica pura) para no romper a
// los consumidores que lo importan desde el componente.
export type { ColumnField } from "@/features/modules/components/action-form-dialog.logic";

/**
 * Clases Tailwind del desplegable de un campo "select" en tiempo de ejecución.
 * Son las MISMAS que el `inputClass` compartido del asistente (module-wizard),
 * pero se declaran aquí localmente para NO acoplar el diálogo al wizard.
 */
const selectClass =
  "h-10 w-full rounded-md border border-input-border bg-[var(--input-surface,var(--input))] px-3 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

export interface ActionFormDialogProps {
  readonly open: boolean;
  readonly action: ModuleAction;
  /** Valores iniciales (p. ej. al editar una fila, se precargan sus datos). */
  readonly initialValues?: Record<string, unknown>;
  readonly isPending?: boolean;
  readonly errorMessage?: string | null;
  /**
   * Marca la acción como destructiva: el diálogo asume el rol de confirmación
   * (botón de envío destructivo, icono de advertencia y aviso de
   * irreversibilidad), con paridad frente a `ConfirmActionDialog`. Ausente o
   * `false` = comportamiento idéntico al actual (sin señales de peligro).
   */
  readonly destructive?: boolean;
  /**
   * Si se provee, el formulario se GENERA desde estas columnas (acción editar,
   * Opción B) en vez de `action.inputFields`. Los campos `readOnly` (PK o no
   * editables) se muestran precargados y deshabilitados y NO se incluyen en el
   * `onSubmit`: el padre (module-view) reañade el valor de la PK desde la fila.
   */
  readonly columnFields?: ReadonlyArray<ColumnField>;
  /**
   * SOLO rama inputFields (crear/custom/SELECT): mapa nombre-de-campo → etiqueta
   * legible de la columna. Si falta la clave de un campo, se muestra f.name
   * (retrocompatibilidad). No afecta precarga/validación/submit (siguen por f.name).
   */
  readonly fieldLabels?: Record<string, string>;
  readonly onSubmit: (input: Record<string, unknown>) => void;
  readonly onCancel: () => void;
}

export function ActionFormDialog({
  open,
  action,
  initialValues = {},
  isPending = false,
  errorMessage = null,
  destructive = false,
  columnFields,
  fieldLabels = {},
  onSubmit,
  onCancel,
}: ActionFormDialogProps) {
  const [values, setValues] = React.useState<Record<string, string | boolean>>({});
  const [errors, setErrors] = React.useState<Record<string, string>>({});

  // Modo generado desde columnas (Opción B): el formulario itera sobre
  // columnFields en vez de action.inputFields.
  const columnMode = columnFields !== undefined;

  // Campos visibles en el formulario (modo inputFields): los constantes NO se
  // muestran, inicializan ni validan aquí (su valor literal lo resuelve el
  // backend desde constValue). La semántica vive en la lógica pura.
  const visibleFields = React.useMemo(
    () => getVisibleFields(action.inputFields),
    [action],
  );

  // Inicializa los valores del formulario al abrir, desde initialValues. La
  // construcción del estado inicial (precedencia precargado → defaultValue → "")
  // vive en `buildInitialValues` (lógica pura, testeada en entorno node).
  React.useEffect(() => {
    if (!open) {
      return;
    }
    setValues(
      buildInitialValues({ columnMode, columnFields, visibleFields, initialValues }),
    );
    setErrors({});
  }, [open, columnMode, columnFields, visibleFields, initialValues]);

  React.useEffect(() => {
    if (!open) {
      return;
    }
    function handleKeyDown(event: KeyboardEvent): void {
      if (event.key === "Escape" && !isPending) {
        onCancel();
      }
    }
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [open, isPending, onCancel]);

  if (!open) {
    return null;
  }

  function handleSubmit(event: React.FormEvent): void {
    event.preventDefault();

    // Validación de obligatoriedad y armado del input viven en la lógica pura
    // (`validateRequired`/`buildSubmitInput`), testeadas en entorno node. En
    // columnMode solo participan las columnas editables; los campos constantes ya
    // están fuera de visibleFields.
    const nextErrors = validateRequired({
      columnMode,
      columnFields,
      visibleFields,
      values,
    });
    if (Object.keys(nextErrors).length > 0) {
      setErrors(nextErrors);
      return;
    }
    onSubmit(buildSubmitInput({ columnMode, columnFields, visibleFields, values }));
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
      role="dialog"
      aria-modal="true"
      aria-labelledby="action-form-title"
      onClick={() => {
        if (!isPending) {
          onCancel();
        }
      }}
    >
      <Card
        className="w-full max-w-md shadow-lg"
        onClick={(event) => event.stopPropagation()}
      >
        <CardHeader className={destructive ? "space-y-1" : undefined}>
          <CardTitle
            id="action-form-title"
            className={
              destructive ? "flex items-center gap-2 text-xl" : "text-xl"
            }
          >
            {destructive ? (
              <AlertTriangle
                className="size-5 text-destructive-text"
                aria-hidden="true"
              />
            ) : null}
            {action.label}
          </CardTitle>
          {destructive ? (
            <CardDescription>
              ¿Seguro que deseas ejecutar esta acción? Esta operación puede no ser
              reversible.
            </CardDescription>
          ) : null}
        </CardHeader>

        <form onSubmit={handleSubmit}>
          <CardContent className="space-y-4">
            {errorMessage ? (
              <div
                role="alert"
                className="flex items-start gap-2 rounded-md border border-destructive/50 bg-destructive/10 px-3 py-2 text-sm text-destructive-text"
              >
                <AlertTriangle className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
                <span>{errorMessage}</span>
              </div>
            ) : null}

            {columnMode ? (
              (columnFields ?? []).length === 0 ? (
                <p className="text-sm text-muted-foreground">
                  Esta acción no requiere datos de entrada.
                </p>
              ) : (
                (columnFields ?? []).map((cf) => (
                  <div key={cf.field} className="space-y-1.5">
                    <Label htmlFor={`field_${cf.field}`}>
                      {cf.label}
                      {cf.readOnly ? (
                        <span className="text-muted-foreground"> (solo lectura)</span>
                      ) : cf.required ? (
                        <span className="text-destructive-text"> *</span>
                      ) : null}
                    </Label>
                    {cf.options ? (
                      // Origen lista (estática o dinámica): desplegable con las
                      // opciones. En dinámico el padre inyecta las opciones
                      // cargadas y, vía optionsLoading/optionsError, el estado de
                      // carga (deshabilita + "Cargando…") o de fallo (texto).
                      <>
                        <select
                          id={`field_${cf.field}`}
                          className={selectClass}
                          value={String(values[cf.field] ?? "")}
                          disabled={cf.readOnly || cf.optionsLoading}
                          onChange={(e) =>
                            setValues((prev) => ({ ...prev, [cf.field]: e.target.value }))
                          }
                          aria-invalid={errors[cf.field] ? "true" : undefined}
                        >
                          <option value="" disabled={cf.required}>
                            {cf.optionsLoading ? "Cargando…" : "— Selecciona —"}
                          </option>
                          {cf.options.map((opt) => (
                            <option key={opt.value} value={opt.value}>
                              {opt.label}
                            </option>
                          ))}
                        </select>
                        {cf.optionsError ? (
                          <p className="text-xs text-destructive-text">
                            No se pudieron cargar las opciones
                          </p>
                        ) : null}
                      </>
                    ) : (
                      <Input
                        id={`field_${cf.field}`}
                        type={cf.type === "number" ? "number" : cf.type === "date" ? "date" : "text"}
                        value={String(values[cf.field] ?? "")}
                        disabled={cf.readOnly}
                        onChange={(e) =>
                          setValues((prev) => ({ ...prev, [cf.field]: e.target.value }))
                        }
                        aria-invalid={errors[cf.field] ? "true" : undefined}
                      />
                    )}
                    {errors[cf.field] ? (
                      <p className="mt-1.5 flex items-center gap-1.5 text-xs text-destructive-text">
                        <AlertCircle className="size-3.5 shrink-0" aria-hidden="true" />
                        {errors[cf.field]}
                      </p>
                    ) : null}
                  </div>
                ))
              )
            ) : visibleFields.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                Esta acción no requiere datos de entrada.
              </p>
            ) : (
              visibleFields.map((f) => (
                <div key={f.name} className="space-y-1.5">
                  <Label htmlFor={`field_${f.name}`}>
                    {fieldLabels[f.name] ?? f.name}
                    {f.required ? <span className="text-destructive-text"> *</span> : null}
                  </Label>
                  {f.source === "select" || f.source === "dynamic" ? (
                    <select
                      id={`field_${f.name}`}
                      className={selectClass}
                      value={String(values[f.name] ?? "")}
                      disabled={f.optionsLoading}
                      onChange={(e) =>
                        setValues((prev) => ({ ...prev, [f.name]: e.target.value }))
                      }
                      aria-invalid={errors[f.name] ? "true" : undefined}
                    >
                      <option value="" disabled={f.required}>
                        {f.optionsLoading ? "Cargando…" : "— Selecciona —"}
                      </option>
                      {(f.options ?? []).map((opt) => (
                        <option key={opt.value} value={opt.value}>
                          {opt.label}
                        </option>
                      ))}
                    </select>
                  ) : null}
                  {(f.source === "select" || f.source === "dynamic") &&
                  f.optionsError ? (
                    <p className="text-xs text-destructive-text">
                      No se pudieron cargar las opciones
                    </p>
                  ) : null}
                  {f.source === "select" || f.source === "dynamic" ? null : f.type ===
                    "boolean" ? (
                    <input
                      id={`field_${f.name}`}
                      type="checkbox"
                      checked={Boolean(values[f.name])}
                      onChange={(e) =>
                        setValues((prev) => ({ ...prev, [f.name]: e.target.checked }))
                      }
                      className="size-4 rounded border-input bg-input"
                    />
                  ) : (
                    <Input
                      id={`field_${f.name}`}
                      type={
                        f.type === "number"
                          ? "number"
                          : f.type === "date"
                            ? "date"
                            : "text"
                      }
                      value={String(values[f.name] ?? "")}
                      onChange={(e) =>
                        setValues((prev) => ({ ...prev, [f.name]: e.target.value }))
                      }
                      aria-invalid={errors[f.name] ? "true" : undefined}
                    />
                  )}
                  {errors[f.name] ? (
                    <p className="mt-1.5 flex items-center gap-1.5 text-xs text-destructive-text">
                      <AlertCircle className="size-3.5 shrink-0" aria-hidden="true" />
                      {errors[f.name]}
                    </p>
                  ) : null}
                </div>
              ))
            )}
          </CardContent>

          <CardFooter className="justify-end gap-2">
            <Button type="button" variant="outline" onClick={onCancel} disabled={isPending}>
              Cancelar
            </Button>
            <Button
              type="submit"
              variant={destructive ? "destructive" : "default"}
              disabled={isPending}
            >
              {isPending ? (
                <>
                  <Loader2 className="size-4 animate-spin" aria-hidden="true" />
                  Ejecutando...
                </>
              ) : (
                "Confirmar"
              )}
            </Button>
          </CardFooter>
        </form>
      </Card>
    </div>
  );
}

export default ActionFormDialog;
