import 'package:subdir_root_fixture/src/model/model.dart' as by_package;

import '../model/model.dart' as by_path;

String view() => '${by_package.model()} ${by_path.model()}';
