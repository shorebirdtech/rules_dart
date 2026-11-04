import 'package:subdir_root_fixture/subdir_root_fixture.dart';
import 'package:subdir_root_fixture/src/model/model.dart';

void main() {
  if (view() != 'model model') throw StateError(view());
  if (model() != 'model') throw StateError(model());
}
